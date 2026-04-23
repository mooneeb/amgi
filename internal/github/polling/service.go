package polling

import (
	"context"
	"log/slog"
	"time"

	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/processor/piface"
	"github.com/mooneeb/amgi/internal/store"
)

type GitHubClient interface {
	ListIssues(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error)
	ListPullRequests(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error)
}

type Poller struct {
	logger    *slog.Logger
	ghClient  GitHubClient
	store     *store.Store
	processor piface.ProcessorAPI
	owner     string
	repo      string
	interval  time.Duration
}

func NewPoller(
	logger *slog.Logger,
	ghClient GitHubClient,
	store *store.Store,
	processor piface.ProcessorAPI,
	owner string,
	repo string,
	interval time.Duration,
) *Poller {

	return &Poller{
		logger:    logger,
		ghClient:  ghClient,
		store:     store,
		processor: processor,
		owner:     owner,
		repo:      repo,
		interval:  interval,
	}
}
