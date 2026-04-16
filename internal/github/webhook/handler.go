package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/config/resolve"
	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/filter"
	"github.com/mooneeb/amgi/internal/marvin"
	"github.com/mooneeb/amgi/internal/marvin/miface"
	"github.com/mooneeb/amgi/internal/store"
)

func (wh *webhook) Handler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h := r.Header.Get("X-Hub-Signature-256")
	if h == "" {
		wh.logger.Error("Signature not found", "remote_addr", r.RemoteAddr)
		http.Error(w, "Signature not found", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)

	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		wh.logger.Error("Failed to read body", "error", err)
		return
	}

	if !validateSignature(body, h, wh.secret) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		wh.logger.Error("Invalid signature", "header", h)
		return
	}

	eventTypeRaw := r.Header.Get("X-GitHub-Event")
	et, e, err := normalizePayload(body, eventTypeRaw, wh.logger)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to normalize payload", "error", err)
		return
	}

	if e == nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Info("Unsupported webhook action")
		return
	}

	wh.logger.Info("Webhook received", "event", e)

	owner, repo, err := resolveOwnerRepo(wh.config, e)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to resolve owner and repo", "error", err)
		return
	}

	allowed, actions, err := isActionAllowed(owner, repo, et, e.Action)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to check if action is allowed", "action", e.Action, "error", err)
		return
	}
	if !allowed {
		w.WriteHeader(http.StatusOK)
		wh.logger.Info("Action not allowed", "action", e.Action)
		return
	}

	matched, err := isEventMatch(wh.config, owner, repo, actions, et, e)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to match event", "error", err)
		return
	}

	if !matched {
		w.WriteHeader(http.StatusOK)
		wh.logger.Info("Payload did not match any config filters", "event", e)
		return
	}

	i, err := isIdempotent(wh.store, e)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to check if event is idempotent", "error", err)
		return
	}

	if !i {
		w.WriteHeader(http.StatusOK)
		wh.logger.Info("Event has already been processed", "event", e)
		return
	}

	err = RetryPendingEvents(wh.store, wh.config, wh.marvin, wh.logger)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to retry events pending retry", "error", err)
		return
	}

	mc, err := resolve.ResolveMarvinConfig(wh.config, owner, repo)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to resolve Marvin config", "error", err)
		return
	}

	var apiError *marvin.APIError
	err = wh.marvin.AddTask(mc, e)
	if errors.As(err, &apiError) {
		if apiError.StatusCode == http.StatusBadRequest ||
			apiError.StatusCode == http.StatusUnauthorized ||
			apiError.StatusCode == http.StatusNotFound {

			wh.logger.Error("Failed to create Marvin task", "error", err)
			err = wh.store.Insert(e, store.StoreStatusFailed)
			if err != nil {
				w.WriteHeader(http.StatusOK)
				wh.logger.Error("Failed to insert event into store", "error", err)
				return
			}

		} else {
			wh.logger.Error("Failed to create Marvin task", "error", err)
			err = wh.store.Insert(e, store.StoreStatusPendingRetry)
			if err != nil {
				w.WriteHeader(http.StatusOK)
				wh.logger.Error("Failed to insert event into store", "error", err)
				return
			}
		}
	} else if err != nil {
		wh.logger.Error("Failed to create Marvin task", "error", err)
		err = wh.store.Insert(e, store.StoreStatusPendingRetry)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			wh.logger.Error("Failed to insert event into store", "error", err)
			return
		}
	} else {
		err = wh.store.Insert(e, store.StoreStatusProcessed)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			wh.logger.Error("Marvin Task created succesfully. Failed to insert event into store", "error", err)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	wh.logger.Info("Webhook processed successfully")
}

func validateSignature(
	body []byte,
	signature string,
	secret string,
) bool {

	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write(body)
	es := "sha256=" + hex.EncodeToString(hash.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(es))
}

func normalizePayload(
	body []byte,
	eventTypeRaw string,
	logger *slog.Logger,
) (event.EventType, *event.Event, error) {
	var et event.EventType
	switch eventTypeRaw {
	case "issues":
		et = event.EventTypeIssue
	case "pull_request":
		et = event.EventTypePullRequest
	default:
		return "", nil, fmt.Errorf("invalid event type: %s", eventTypeRaw)
	}
	e, err := NormalizeGithubWebhookPayload(body, et, logger)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse payload: %w", err)
	}
	return et, e, nil
}

func resolveOwnerRepo(
	cfg *config.Config,
	e *event.Event,
) (*config.Owner, *config.Repository, error) {
	owner, err := resolve.ResolveOwner(cfg, e.Owner)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve owner: %w", err)
	}
	repo, err := resolve.ResolveRepository(owner, e.Repo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve repository: %w", err)
	}
	return owner, repo, nil
}

func isActionAllowed(
	owner *config.Owner,
	repo *config.Repository,
	et event.EventType,
	eventAction event.EventAction,
) (bool, []string, error) {
	actions, err := resolve.ResolveActions(owner, repo, et)
	if err != nil {
		return false, nil, fmt.Errorf("failed to resolve actions: %w", err)
	}
	switch et {
	case event.EventTypeIssue:
		return slices.Contains(actions, string(eventAction)), actions, nil
	case event.EventTypePullRequest:
		return slices.Contains(actions, string(eventAction)), actions, nil
	}
	return false, nil, fmt.Errorf("no actions found for event type %s", et)
}

func isEventMatch(
	cfg *config.Config,
	owner *config.Owner,
	repo *config.Repository,
	actions []string,
	et event.EventType,
	e *event.Event,
) (bool, error) {
	matched := false
	filters, err := resolve.ResolveFilters(cfg, owner, repo)
	if err != nil {
		return matched, fmt.Errorf("failed to resolve filters: %w", err)
	}
	if filters == nil {
		return true, nil
	}
	switch et {
	case event.EventTypeIssue:
		matched, err = filter.IsIssueMatch(e, filters.Issues, actions)
		if err != nil {
			return matched, fmt.Errorf("failed to match issue: %w", err)
		}
	case event.EventTypePullRequest:
		matched, err = filter.IsPullRequestMatch(e, filters.PullRequests, actions)
		if err != nil {
			return matched, fmt.Errorf("failed to match pull request: %w", err)
		}
	}
	return matched, nil
}

func isIdempotent(store *store.Store, e *event.Event) (bool, error) {
	exists, err := store.HasEvent(e.Owner, e.Repo, e.Number)
	if err != nil {
		return false, fmt.Errorf("failed to check if event exists: %w", err)
	}
	if exists {
		return false, nil
	}
	return true, nil
}

func RetryPendingEvents(
	st *store.Store,
	cfg *config.Config,
	marvinAPI miface.MarvinAPI,
	logger *slog.Logger,
) error {
	events, err := st.GetPendingRetryEvents(config.MaxRetryAttempts)
	if err != nil {
		return fmt.Errorf("failed to get pending retry events: %w", err)
	}
	for _, e := range events {
		owner, repo, err := resolveOwnerRepo(cfg, e.Event)
		if err != nil {
			return fmt.Errorf("failed to resolve owner and repo: %w", err)
		}
		mc, err := resolve.ResolveMarvinConfig(cfg, owner, repo)
		if err != nil {
			return fmt.Errorf("failed to resolve Marvin config: %w", err)
		}
		err = marvinAPI.AddTask(mc, e.Event)
		if err != nil {
			var ss store.StoreStatus
			retryCount := e.RetryCount + 1
			err = st.IncrementRetryCount(e.Event.Owner, e.Event.Repo, e.Event.Number)
			if err != nil {
				logger.Error("failed to increment retry count", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number)
			}
			if retryCount >= config.MaxRetryAttempts {
				ss = store.StoreStatusFailed
			} else {
				ss = store.StoreStatusPendingRetry
			}
			err = st.MarkAs(e.Event.Owner, e.Event.Repo, e.Event.Number, ss)
			if err != nil {
				logger.Error("failed to mark as", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number, "status", ss)
			}
			continue
		}

		err = st.MarkAs(e.Event.Owner, e.Event.Repo, e.Event.Number, store.StoreStatusProcessed)
		if err != nil {
			logger.Error("failed to mark as processed", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number)
		}
		logger.Info("Event retried successfully", "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number)
	}
	return nil
}
