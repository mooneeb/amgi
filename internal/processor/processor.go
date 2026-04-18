package processor

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/config/resolve"
	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/filter"
	"github.com/mooneeb/amgi/internal/marvin"
	"github.com/mooneeb/amgi/internal/store"
)

func (p *processor) Process(
	ctx context.Context,
	e *event.Event,
) error {
	owner, repo, err := resolveOwnerRepo(p.cfg, e)
	if err != nil {
		return fmt.Errorf("failed to resolve owner and repo: %w", err)
	}
	matched, err := isEventMatch(p.cfg, owner, repo, event.EventType(e.Type), e)
	if err != nil {
		return fmt.Errorf("failed to match event: %w", err)
	}

	if !matched {
		p.logger.Info("Payload did not match any config filters", "event", e)
		return nil
	}

	i, err := isIdempotent(p.store, e)
	if err != nil {
		return fmt.Errorf("failed to check if event is idempotent: %w", err)
	}

	if !i {
		p.logger.Info("Event has already been processed", "event", e)
		return nil
	}

	mc, err := resolve.ResolveMarvinConfig(p.cfg, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to resolve Marvin config: %w", err)
	}

	var apiError *marvin.APIError
	err = p.marvinAPI.AddTask(mc, e)
	if errors.As(err, &apiError) {
		if apiError.StatusCode == http.StatusBadRequest ||
			apiError.StatusCode == http.StatusUnauthorized ||
			apiError.StatusCode == http.StatusNotFound {
			err = p.store.Insert(e, store.StoreStatusFailed)
			if err != nil {
				return fmt.Errorf("failed to insert event into store: %w", err)
			}

		} else {
			err = p.store.Insert(e, store.StoreStatusPendingRetry)
			if err != nil {
				return fmt.Errorf("failed to insert event into store: %w", err)
			}
		}
	} else {
		err = p.store.Insert(e, store.StoreStatusProcessed)
		if err != nil {
			return fmt.Errorf("failed to insert event into store: %w", err)
		}
	}
	return nil
}

func (p *processor) RetryPending(
	ctx context.Context,
) error {
	events, err := p.store.GetPendingRetryEvents(config.MaxRetryAttempts)
	if err != nil {
		return fmt.Errorf("failed to get pending retry events: %w", err)
	}
	for _, e := range events {
		owner, repo, err := resolveOwnerRepo(p.cfg, e.Event)
		if err != nil {
			return fmt.Errorf("failed to resolve owner and repo: %w", err)
		}
		mc, err := resolve.ResolveMarvinConfig(p.cfg, owner, repo)
		if err != nil {
			return fmt.Errorf("failed to resolve Marvin config: %w", err)
		}
		err = p.marvinAPI.AddTask(mc, e.Event)
		if err != nil {
			var ss store.StoreStatus
			retryCount := e.RetryCount + 1
			err = p.store.IncrementRetryCount(e.Event.Owner, e.Event.Repo, e.Event.Number)
			if err != nil {
				p.logger.Error("failed to increment retry count", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number)
			}
			if retryCount >= config.MaxRetryAttempts {
				ss = store.StoreStatusFailed
			} else {
				ss = store.StoreStatusPendingRetry
			}
			err = p.store.MarkAs(e.Event.Owner, e.Event.Repo, e.Event.Number, ss)
			if err != nil {
				p.logger.Error("failed to mark as", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number, "status", ss)
			}
			continue
		}

		err = p.store.MarkAs(e.Event.Owner, e.Event.Repo, e.Event.Number, store.StoreStatusProcessed)
		if err != nil {
			p.logger.Error("failed to mark as processed", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number)
		}
		p.logger.Info("Event retried successfully", "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number)
	}
	return nil
}

func resolveOwnerRepo(
	cfg *config.Config,
	e *event.Event,
) (*config.Owner, *config.Repository, error) {
	owner, err := resolve.ResolveOwner(cfg, e.Owner)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve owner: %w", err)
	}
	repo, err := resolve.ResolveRepository(owner, e.Repo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve repository: %w", err)
	}
	return owner, repo, nil
}

func isEventMatch(
	cfg *config.Config,
	owner *config.Owner,
	repo *config.Repository,
	et event.EventType,
	e *event.Event,
) (bool, error) {
	matched := false
	filters, err := resolve.ResolveFilters(cfg, owner, repo)
	if err != nil {
		return matched, fmt.Errorf("failed to resolve filters: %w", err)
	}
	if filters == nil {
		return true, nil
	}
	switch et {
	case event.EventTypeIssue:
		matched, err = filter.IsIssueMatch(e, filters.Issues)
		if err != nil {
			return matched, fmt.Errorf("failed to match issue: %w", err)
		}
	case event.EventTypePullRequest:
		matched, err = filter.IsPullRequestMatch(e, filters.PullRequests)
		if err != nil {
			return matched, fmt.Errorf("failed to match pull request: %w", err)
		}
	}
	return matched, nil
}

func isIdempotent(store *store.Store, e *event.Event) (bool, error) {
	exists, err := store.HasEvent(e.Owner, e.Repo, e.Number)
	if err != nil {
		return false, fmt.Errorf("failed to check if event exists: %w", err)
	}
	if exists {
		return false, nil
	}
	return true, nil
}
