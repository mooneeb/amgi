package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gogithub "github.com/google/go-github/v84/github"
	"github.com/mooneeb/amgi/internal/event"
	"golang.org/x/oauth2"
)

type Client struct {
	logger *slog.Logger
	client *gogithub.Client
}

func New(
	logger *slog.Logger,
	apiToken string,
) *Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: apiToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	return &Client{
		logger: logger,
		client: gogithub.NewClient(tc),
	}
}

func (c *Client) ListPullRequests(
	ctx context.Context,
	owner, repo string,
	since time.Time,
) ([]*event.Event, error) {
	opts := &gogithub.PullRequestListOptions{
		State:     "open",
		Sort:      "created",
		Direction: "desc",
		ListOptions: gogithub.ListOptions{
			PerPage: 100,
		},
	}
	var events []*event.Event
	var done bool
	for !done {
		pullRequests, resp, err := c.client.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			var rlErr *gogithub.RateLimitError
			if !errors.As(err, &rlErr) {
				return nil, fmt.Errorf("ListPullRequests %s/%s: %w", owner, repo, err)
			}
			slp := time.Until(rlErr.Rate.Reset.Time)
			if slp > 5*time.Minute {
				slp = 5 * time.Minute
			}
			c.logger.Warn("ListPullRequests: Github Rate limit exceeded",
				"owner", owner, "repo", repo,
				"error", err, "sleep", slp,
				"reset", rlErr.Rate.Reset.Time,
			)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(slp):
			}

			pullRequests, resp, err = c.client.PullRequests.List(ctx, owner, repo, opts)
			if err != nil {
				return nil, fmt.Errorf("ListPullRequests %s/%s After rate limit exceeded: %w", owner, repo, err)
			}
		}

		for _, sdkPullRequest := range pullRequests {
			if sdkPullRequest.GetCreatedAt().Before(since) {
				done = true
				break
			}
			pullRequest := fromSDKPullRequest(sdkPullRequest)
			e, err := NormalizeGithubPullRequestPayload(pullRequest, owner, repo, event.EventActionOpened)
			if err != nil {
				return nil, fmt.Errorf("ListPullRequests %s/%s: %w", owner, repo, err)
			}
			events = append(events, e)
		}
		if resp.NextPage == 0 {
			done = true
		}
		if !done {
			opts.ListOptions.Page = resp.NextPage
		}
	}

	return events, nil
}

func (c *Client) ListIssues(
	ctx context.Context,
	owner, repo string,
	since time.Time,
) ([]*event.Event, error) {
	opts := &gogithub.IssueListByRepoOptions{
		State:     "open",
		Sort:      "created",
		Direction: "desc",
		Since:     since,
		ListOptions: gogithub.ListOptions{
			PerPage: 100,
		},
	}
	var events []*event.Event
	var done bool
	for !done {
		issues, resp, err := c.client.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			var rlErr *gogithub.RateLimitError
			if !errors.As(err, &rlErr) {
				return nil, fmt.Errorf("ListIssues %s/%s: %w", owner, repo, err)
			}
			slp := time.Until(rlErr.Rate.Reset.Time)
			if slp > 5*time.Minute {
				slp = 5 * time.Minute
			}
			c.logger.Warn("ListIssues: Github Rate limit exceeded",
				"owner", owner, "repo", repo,
				"error", err, "sleep", slp,
				"reset", rlErr.Rate.Reset.Time,
			)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(slp):
			}

			issues, resp, err = c.client.Issues.ListByRepo(ctx, owner, repo, opts)
			if err != nil {
				return nil, fmt.Errorf("ListIssues %s/%s After rate limit exceeded: %w", owner, repo, err)
			}
		}

		for _, sdkIssue := range issues {
			if sdkIssue.IsPullRequest() {
				continue
			}
			if sdkIssue.GetCreatedAt().Before(since) {
				done = true
				break
			}
			issue := fromSDKIssue(sdkIssue)
			e, err := NormalizeGithubIssuePayload(issue, owner, repo, event.EventActionOpened)
			if err != nil {
				return nil, fmt.Errorf("ListIssues %s/%s: %w", owner, repo, err)
			}
			events = append(events, e)
		}

		if resp.NextPage == 0 {
			done = true
		}

		if !done {
			opts.ListOptions.Page = resp.NextPage
		}
	}

	return events, nil
}

func fromSDKPullRequest(p *gogithub.PullRequest) PullRequest {
	return PullRequest{
		Number:    p.GetNumber(),
		Title:     p.GetTitle(),
		Body:      p.Body,
		State:     p.GetState(),
		Labels:    fromSDKLabels(p.Labels),
		Assignees: fromSDKUsers(p.Assignees),
		URL:       p.GetHTMLURL(),
		User:      fromSDKUser(p.User),
		Head:      githubBranch{Ref: p.Head.GetRef()},
		Reviewers: fromSDKUsers(p.RequestedReviewers),
	}
}

func fromSDKIssue(i *gogithub.Issue) Issue {
	return Issue{
		Number:    i.GetNumber(),
		Title:     i.GetTitle(),
		Body:      i.Body,
		State:     i.GetState(),
		Labels:    fromSDKLabels(i.Labels),
		Assignees: fromSDKUsers(i.Assignees),
		URL:       i.GetHTMLURL(),
		User:      fromSDKUser(i.User),
	}
}

func fromSDKLabels(labels []*gogithub.Label) []githubLabel {
	var ll []githubLabel
	for _, label := range labels {
		ll = append(ll, githubLabel{Name: label.GetName()})
	}
	return ll
}

func fromSDKUsers(users []*gogithub.User) []githubUser {
	var uu []githubUser
	for _, user := range users {
		uu = append(uu, githubUser{Login: user.GetLogin()})
	}
	return uu
}

func fromSDKUser(u *gogithub.User) githubUser {
	if u == nil {
		return githubUser{}
	}
	return githubUser{Login: u.GetLogin()}
}
