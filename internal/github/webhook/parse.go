package webhook

import (
	"encoding/json"
	"fmt"
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
) (*event.Event, error) {
	switch eventType {
	case event.EventTypeIssue:
		var wh githubIssueWebhook
		err := json.Unmarshal(payload, &wh)
		if err != nil {
			return nil, err
		}
		org, repo, err := resolveOrgAndRepo(wh.Repository)
		if err != nil {
			return nil, err
		}
		action, err := getWebhookAction(wh.Action)
		if err != nil {
			return nil, err
		}
		return igithub.NormalizeGithubIssuePayload(wh.Issue, org, repo, action)
	case event.EventTypePullRequest:
		var wh githubPullRequestWebhook
		err := json.Unmarshal(payload, &wh)
		if err != nil {
			return nil, err
		}
		org, repo, err := resolveOrgAndRepo(wh.Repository)
		if err != nil {
			return nil, err
		}
		action, err := getWebhookAction(wh.Action)
		if err != nil {
			return nil, err
		}
		return igithub.NormalizeGithubPullRequestPayload(wh.PullRequest, org, repo, action)
	default:
		return nil, fmt.Errorf("invalid event type: %s", string(eventType))
	}
}

func getWebhookAction(action string) (event.EventAction, error) {
	if action == "opened" {
		return event.EventActionOpened, nil
	} else if action == "assigned" {
		return event.EventActionAssigned, nil
	} else if action == "review_requested" {
		return event.EventActionReviewRequested, nil
	}
	return "", fmt.Errorf("invalid webhook action: %s", action)
}

func resolveOrgAndRepo(
	r igithub.Repository,
) (string, string, error) {
	parts := strings.Split(r.FullName, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository full name: %s", r.FullName)
	}
	return parts[0], parts[1], nil
}
