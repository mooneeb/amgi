package github

import (
	"context"
	"log/slog"

	"github.com/google/go-github/v50/github"
	"golang.org/x/oauth2"
)

type Github struct {
	logger   *slog.Logger
	client   *github.Client
	apiToken *string
}

func New(
	logger *slog.Logger,
	apiToken *string,
) Github {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *apiToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return Github{
		logger:   logger,
		client:   github.NewClient(tc),
		apiToken: apiToken,
	}
}

func (g *Github) GetIssue(owner, repo string, number int) (*github.Issue, error) {
	issue, _, err := g.client.Issues.Get(context.Background(), owner, repo, number)
	if err != nil {
		return nil, err
	}
	return issue, nil
}
