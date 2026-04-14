package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/config/resolve"
	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/filter"
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
	et, e, err := normalizePayload(body, eventTypeRaw)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to normalize payload", "error", err)
		return
	}

	wh.logger.Info("Webhook received", "event", e)

	org, repo, err := resolveOrgRepo(wh.config, e)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to resolve org and repo", "error", err)
		return
	}

	allowed, actions, err := isActionAllowed(org, repo, et, e.Action)
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

	matched, err := isEventMatch(wh.config, org, repo, actions, et, e)
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

	err = wh.store.Retry()
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to retry events pending retry", "error", err)
		return
	}

	mc, err := resolve.ResolveMarvinConfig(wh.config, org, repo)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to resolve Marvin config", "error", err)
		return
	}

	err = createMarvinTask(mc)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to create Marvin task", "error", err)
		return
	}

	err = wh.store.Insert(e, store.StoreStatusProcessed)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Marvin Task created succesfully. Failed to insert event into store", "error", err)
		return
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
	e, err := NormalizeGithubPayload(body, et)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse payload: %w", err)
	}
	return et, e, nil
}

func resolveOrgRepo(
	config *config.Config,
	e *event.Event,
) (*config.Organization, *config.Repository, error) {
	org, err := resolve.ResolveOrganization(config, e.Org)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve organization: %w", err)
	}
	repo, err := resolve.ResolveRepository(org, e.Repo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve repository: %w", err)
	}
	return org, repo, nil
}

func isActionAllowed(
	org *config.Organization,
	repo *config.Repository,
	et event.EventType,
	eventAction event.EventAction,
) (bool, []string, error) {
	actions, err := resolve.ResolveActions(org, repo, et)
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
	config *config.Config,
	org *config.Organization,
	repo *config.Repository,
	actions []string,
	et event.EventType,
	e *event.Event,
) (bool, error) {
	matched := false
	filters, err := resolve.ResolveFilters(config, org, repo)
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
	exists, err := store.HasEvent(e.Org, e.Repo, e.Number)
	if err != nil {
		return false, fmt.Errorf("failed to check if event exists: %w", err)
	}
	if exists {
		return false, nil
	}
	return true, nil
}

func createMarvinTask(mc *config.MarvinConfig) error {
	return nil
}
