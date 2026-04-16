package webhook

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mooneeb/amgi/internal/event"
	igithub "github.com/mooneeb/amgi/internal/github"
)

type githubPullRequestWebhook struct {
	Repository  igithub.Repository  `json:"repository"`
	PullRequest igithub.PullRequest `json:"pull_request"`
	Action      string              `json:"action"`
}

type githubIssueWebhook struct {
	Repository igithub.Repository `json:"repository"`
	Issue      igithub.Issue      `json:"issue"`
	Action     string             `json:"action"`
}

func NormalizeGithubWebhookPayload(
	payload []byte,
	eventType event.EventType,
	logger *slog.Logger,
) (*event.Event, error) {
	switch eventType {
	case event.EventTypeIssue:
		var wh githubIssueWebhook
		err := json.Unmarshal(payload, &wh)
		if err != nil {
			return nil, err
		}
		owner, repo, err := resolveOwnerAndRepo(wh.Repository)
		if err != nil {
			return nil, err
		}
		action := getWebhookAction(wh.Action)
		if action == "" {
			logger.Info("Unsupported webhook action", "action", wh.Action)
			return nil, nil
		}
		return igithub.NormalizeGithubIssuePayload(wh.Issue, owner, repo, action)
	case event.EventTypePullRequest:
		var wh githubPullRequestWebhook
		err := json.Unmarshal(payload, &wh)
		if err != nil {
			return nil, err
		}
		owner, repo, err := resolveOwnerAndRepo(wh.Repository)
		if err != nil {
			return nil, err
		}
		action := getWebhookAction(wh.Action)
		if action == "" {
			logger.Info("Unsupported webhook action", "action", wh.Action)
			return nil, nil
		}
		return igithub.NormalizeGithubPullRequestPayload(wh.PullRequest, owner, repo, action)
	default:
		return nil, fmt.Errorf("invalid event type: %s", string(eventType))
	}
}

func getWebhookAction(action string) event.EventAction {
	if action == "opened" {
		return event.EventActionOpened
	} else if action == "assigned" {
		return event.EventActionAssigned
	} else if action == "review_requested" {
		return event.EventActionReviewRequested
	}
	return event.EventAction("")
}

func resolveOwnerAndRepo(
	r igithub.Repository,
) (string, string, error) {
	parts := strings.Split(r.FullName, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository full name: %s", r.FullName)
	}
	return parts[0], parts[1], nil
}
