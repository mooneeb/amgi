package polling

import (
	"context"
	"fmt"
	"time"
)

func (p *Poller) Run(ctx context.Context) error {

	p.tick(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.tick(ctx)
		case <-ctx.Done():
			p.logger.Info("poller stopped", "owner", p.owner, "repo", p.repo)
			return nil
		}
	}

}

func (p *Poller) tick(ctx context.Context) error {
	since, found, err := p.store.GetPollCursor(p.owner, p.repo)
	if err != nil {
		return fmt.Errorf("failed to get poll cursor: %w", err)
	}
	if !found {
		p.logger.Info("no poll cursor found. First poll", "owner", p.owner, "repo", p.repo)
		since = time.Now().UTC()
	}

	nextCursor := time.Now().UTC()

	issues, err := p.ghClient.ListIssues(ctx, p.owner, p.repo, since)
	if err != nil {
		return fmt.Errorf("failed to list issues: %w", err)
	}

	pullRequests, err := p.ghClient.ListPullRequests(ctx, p.owner, p.repo, since)
	if err != nil {
		return fmt.Errorf("failed to list pull requests: %w", err)
	}

	err = p.store.UpsertPollCursor(p.owner, p.repo, nextCursor)
	if err != nil {
		return fmt.Errorf("failed to upsert poll cursor: %w", err)
	}

	for _, issue := range issues {
		err = p.processor.Process(ctx, issue)
		if err != nil {
			p.logger.Error("failed to process issue", "owner", p.owner, "repo", p.repo, "issue number", issue.Number, "error", err)
			continue
		}
	}

	for _, pullRequest := range pullRequests {
		err = p.processor.Process(ctx, pullRequest)
		if err != nil {
			p.logger.Error("failed to process pull request", "owner", p.owner, "repo", p.repo, "pull request number", pullRequest.Number, "error", err)
			continue
		}
	}

	p.logger.Info("polled successfully", "owner", p.owner, "repo", p.repo, "issue count", len(issues), "pull request count", len(pullRequests), "since", since, "nextCursor", nextCursor)

	return nil
}
