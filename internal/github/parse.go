package github

import (
	"github.com/mooneeb/amgi/internal/event"
)

type Issue struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Body      *string       `json:"body"`
	State     string        `json:"state"`
	Labels    []githubLabel `json:"labels"`
	Assignees []githubUser  `json:"assignees"`
	URL       string        `json:"html_url"`
	User      githubUser    `json:"user"`
}

type PullRequest struct {
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

type Repository struct {
	FullName string `json:"full_name"`
}

type githubLabel struct {
	Name string `json:"name"`
}

type githubUser struct {
	Login string `json:"login"`
}

func NormalizeGithubIssuePayload(
	payload Issue,
	owner, repo string,
	action event.EventAction,
) (*event.Event, error) {
	var e *event.Event
	body := ""
	if payload.Body != nil {
		body = *payload.Body
	}
	e = &event.Event{
		Type:      string(event.EventTypeIssue),
		Owner:     owner,
		Repo:      repo,
		Number:    payload.Number,
		Title:     payload.Title,
		Body:      body,
		State:     payload.State,
		Labels:    extractStrings(payload.Labels, func(l githubLabel) string { return l.Name }),
		Assignees: extractStrings(payload.Assignees, func(u githubUser) string { return u.Login }),
		Author:    payload.User.Login,
		URL:       payload.URL,
		Action:    action,
	}

	return e, nil
}

func NormalizeGithubPullRequestPayload(
	payload PullRequest,
	owner, repo string,
	action event.EventAction,
) (*event.Event, error) {
	var e *event.Event
	body := ""
	if payload.Body != nil {
		body = *payload.Body
	}
	e = &event.Event{
		Type:      string(event.EventTypePullRequest),
		Owner:     owner,
		Repo:      repo,
		Number:    payload.Number,
		Title:     payload.Title,
		Body:      body,
		State:     payload.State,
		Labels:    extractStrings(payload.Labels, func(l githubLabel) string { return l.Name }),
		Assignees: extractStrings(payload.Assignees, func(u githubUser) string { return u.Login }),
		Branch:    payload.Head.Ref,
		Reviewers: extractStrings(payload.Reviewers, func(u githubUser) string { return u.Login }),
		Author:    payload.User.Login,
		URL:       payload.URL,
		Action:    action,
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
