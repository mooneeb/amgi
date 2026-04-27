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
	owner, repo, err := resolve.ResolveOwnerRepo(p.cfg, e.Owner, e.Repo)
	if err != nil {
		// Not a processing failure: the event is for a repo we are not
		// configured to track. Skip consistent with filter-miss and
		// duplicate-event handling below (log and return nil).
		p.logger.Warn("event skipped: owner or repo not in config",
			"owner", e.Owner, "repo", e.Repo, "number", e.Number, "error", err)
		return nil
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

	err = p.marvinAPI.AddTask(ctx, mc, e)
	if err == nil {
		err = p.store.Insert(e, store.StoreStatusProcessed)
		if err != nil {
			return fmt.Errorf("failed to insert event into store: %w", err)
		}
		p.logger.Info("task created",
			"owner", owner.Name, "repo", repo.Name,
			"number", e.Number, "type", e.Type,
			"mode", owner.Mode)
		return nil
	}

	var (
		budgetErr *marvin.DailyBudgetExceededError
		apiErr    *marvin.APIError
	)

	switch {
	case errors.As(err, &budgetErr):
		p.logger.Warn("daily budget exceeded", "resets_at", budgetErr.ResetsAt, "owner", owner.Name, "repo", repo.Name, "number", e.Number)
		err = p.store.Insert(e, store.StoreStatusPendingRetry)
		if err != nil {
			return fmt.Errorf("failed to insert event into store: %w", err)
		}
	case errors.As(err, &apiErr):
		if isPermanentAPIError(apiErr) {
			p.logger.Error("permanent API error", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Number)
			err = p.store.Insert(e, store.StoreStatusFailed)
			if err != nil {
				return fmt.Errorf("failed to insert event into store: %w", err)
			}
		} else {
			p.logger.Warn("non-permanent API error. setting event to pending retry", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Number)
			err = p.store.Insert(e, store.StoreStatusPendingRetry)
			if err != nil {
				return fmt.Errorf("failed to insert event into store: %w", err)
			}
		}
	default:
		p.logger.Warn("unknown error. setting event to pending retry", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Number)
		err = p.store.Insert(e, store.StoreStatusPendingRetry)
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
	if len(events) == 0 {
		return nil
	}
	p.logger.Info("retry pass starting", "pending_retry_count", len(events))
	for _, e := range events {
		owner, repo, err := resolve.ResolveOwnerRepo(p.cfg, e.Event.Owner, e.Event.Repo)
		if err != nil {
			p.logger.Warn("retry skipped: failed to resolve owner/repo",
				"error", err, "owner", e.Event.Owner, "repo", e.Event.Repo, "number", e.Event.Number)
			continue
		}
		mc, err := resolve.ResolveMarvinConfig(p.cfg, owner, repo)
		if err != nil {
			p.logger.Warn("retry skipped: failed to resolve Marvin config",
				"error", err, "owner", e.Event.Owner, "repo", e.Event.Repo, "number", e.Event.Number)
			continue
		}
		var budgetErr *marvin.DailyBudgetExceededError
		err = p.marvinAPI.AddTask(ctx, mc, e.Event)
		if err != nil {
			if errors.As(err, &budgetErr) {
				p.logger.Warn("retry skipped: marvin daily budget exhausted",
					"resets_at", budgetErr.ResetsAt, "owner", e.Event.Owner, "repo", e.Event.Repo, "number", e.Event.Number)
				continue
			}
			var ss store.StoreStatus
			retryCount := e.RetryCount + 1
			if retryCount >= config.MaxRetryAttempts {
				ss = store.StoreStatusFailed
			} else {
				ss = store.StoreStatusPendingRetry
			}
			p.logger.Warn("retry attempt failed",
				"error", err,
				"owner", owner.Name, "repo", repo.Name, "number", e.Event.Number,
				"retry_count", retryCount, "new_status", ss)

			err = p.store.IncrementRetryCount(e.Event.Owner, e.Event.Repo, e.Event.Number)
			if err != nil {
				p.logger.Error("failed to increment retry count", "error", err, "owner", owner.Name, "repo", repo.Name, "number", e.Event.Number)
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

func isEventMatch(
	cfg *config.Config,
	owner *config.Owner,
	repo *config.Repository,
	et event.EventType,
	e *event.Event,
) (bool, error) {
	matched := false
	filters := resolve.ResolveFilters(cfg, owner, repo)
	if filters == nil {
		return true, nil
	}
	var err error
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

// isPermanentAPIError returns true if the API error should not be retried.
func isPermanentAPIError(apiErr *marvin.APIError) bool {
	switch apiErr.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound:
		return true
	default:
		return false
	}
}
