package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/config/resolve"
	"github.com/mooneeb/amgi/internal/event"
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

	owner, repo, err := resolve.ResolveOwnerRepo(wh.config, e.Owner, e.Repo)
	if err != nil {
		// Not a failure: the webhook fired for a repo this AMGI instance
		// isn't configured to track (stale webhook, config reshuffle, etc).
		// Acknowledge to GitHub so no retry is scheduled, and continue.
		w.WriteHeader(http.StatusOK)
		wh.logger.Warn("event skipped: owner or repo not in config",
			"owner", e.Owner, "repo", e.Repo, "error", err)
		return
	}

	allowed, err := isActionAllowed(owner, repo, et, e.Action)
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

	err = wh.processor.Process(r.Context(), e)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		wh.logger.Error("Failed to process event", "error", err, "event", e, "owner", owner.Name, "repo", repo.Name)
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

func isActionAllowed(
	owner *config.Owner,
	repo *config.Repository,
	et event.EventType,
	eventAction event.EventAction,
) (bool, error) {
	actions, err := resolve.ResolveActions(owner, repo, et)
	if err != nil {
		return false, fmt.Errorf("failed to resolve actions: %w", err)
	}
	switch et {
	case event.EventTypeIssue:
		return slices.Contains(actions, string(eventAction)), nil
	case event.EventTypePullRequest:
		return slices.Contains(actions, string(eventAction)), nil
	}
	return false, fmt.Errorf("no actions found for event type %s", et)
}
