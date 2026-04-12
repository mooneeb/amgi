package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/mooneeb/amgi/internal/event"
)

func (wh *webhook) Handler(w http.ResponseWriter, r *http.Request) {
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
	var et event.EventType
	switch eventTypeRaw {
	case "issues":
		et = event.EventTypeIssue
	case "pull_request":
		et = event.EventTypePullRequest
	default:
		http.Error(w, "Invalid event type", http.StatusBadRequest)
		wh.logger.Error("Invalid event type", "event_type", eventTypeRaw)
		return
	}
	e, err := NormalizeGithubPayload(body, et)
	if err != nil {
		http.Error(w, "Failed to parse payload", http.StatusInternalServerError)
		wh.logger.Error("Failed to parse payload", "error", err)
		return
	}
	wh.logger.Info("Webhook received", "event", e)

	w.WriteHeader(http.StatusOK)
}

func validateSignature(body []byte, signature string, secret string) bool {

	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write(body)
	es := "sha256=" + hex.EncodeToString(hash.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(es))
}
