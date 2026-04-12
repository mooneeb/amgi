package webhook

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mooneeb/amgi/internal/event"
)

type githubIssueWebhook struct {
	Repository githubRepository `json:"repository"`
	Issue      githubIssue      `json:"issue"`
	Action     string           `json:"action"`
}

type githubIssue struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Body      *string       `json:"body"`
	State     string        `json:"state"`
	Labels    []githubLabel `json:"labels"`
	Assignees []githubUser  `json:"assignees"`
	URL       string        `json:"html_url"`
	User      githubUser    `json:"user"`
}

type githubPullRequestWebhook struct {
	Repository  githubRepository  `json:"repository"`
	PullRequest githubPullRequest `json:"pull_request"`
	Action      string            `json:"action"`
}

type githubPullRequest struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Body      *string       `json:"body"`
	State     string        `json:"state"`
	Labels    []githubLabel `json:"labels"`
	Assignees []githubUser  `json:"assignees"`
	URL       string        `json:"html_url"`
	User      githubUser    `json:"user"`
	Head      githubBranch  `json:"head"`
	Reviewers []githubUser  `json:"requested_reviewers"`
}

type githubBranch struct {
	Ref string `json:"ref"`
}

type githubRepository struct {
	FullName string `json:"full_name"`
}

type githubLabel struct {
	Name string `json:"name"`
}

type githubUser struct {
	Login string `json:"login"`
}

func NormalizeGithubPayload(payload []byte, eventType event.EventType) (*event.Event, error) {
	var e *event.Event
	if eventType == event.EventTypeIssue {
		var wh githubIssueWebhook
		err := json.Unmarshal(payload, &wh)
		if err != nil {
			return nil, err
		}

		parts := strings.Split(wh.Repository.FullName, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repository full name: %s", wh.Repository.FullName)
		}
		body := ""
		if wh.Issue.Body != nil {
			body = *wh.Issue.Body
		}
		e = &event.Event{
			Type:      string(eventType),
			Org:       parts[0],
			Repo:      parts[1],
			Number:    wh.Issue.Number,
			Title:     wh.Issue.Title,
			Body:      body,
			State:     wh.Issue.State,
			Labels:    extractStrings(wh.Issue.Labels, func(l githubLabel) string { return l.Name }),
			Assignees: extractStrings(wh.Issue.Assignees, func(u githubUser) string { return u.Login }),
			Author:    wh.Issue.User.Login,
			URL:       wh.Issue.URL,
			Action:    event.EventActionOpened,
		}
	} else if eventType == event.EventTypePullRequest {
		var wh githubPullRequestWebhook
		err := json.Unmarshal(payload, &wh)
		if err != nil {
			return nil, err
		}

		parts := strings.Split(wh.Repository.FullName, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repository full name: %s", wh.Repository.FullName)
		}

		body := ""
		if wh.PullRequest.Body != nil {
			body = *wh.PullRequest.Body
		}
		e = &event.Event{
			Type:      string(eventType),
			Org:       parts[0],
			Repo:      parts[1],
			Number:    wh.PullRequest.Number,
			Title:     wh.PullRequest.Title,
			Body:      body,
			State:     wh.PullRequest.State,
			Labels:    extractStrings(wh.PullRequest.Labels, func(l githubLabel) string { return l.Name }),
			Assignees: extractStrings(wh.PullRequest.Assignees, func(u githubUser) string { return u.Login }),
			Author:    wh.PullRequest.User.Login,
			Branch:    wh.PullRequest.Head.Ref,
			Reviewers: extractStrings(wh.PullRequest.Reviewers, func(u githubUser) string { return u.Login }),
			URL:       wh.PullRequest.URL,
			Action:    event.EventActionReviewRequested,
		}
	} else {
		return nil, fmt.Errorf("invalid event type: %s", string(eventType))
	}

	return e, nil
}

func extractStrings[T any](items []T, fn func(T) string) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = fn(item)
	}
	return result
}
