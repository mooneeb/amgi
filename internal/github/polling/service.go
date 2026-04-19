package polling

import (
	"log/slog"
	"time"

	igithub "github.com/mooneeb/amgi/internal/github"
	"github.com/mooneeb/amgi/internal/processor/piface"
	"github.com/mooneeb/amgi/internal/store"
)

type Poller struct {
	logger    *slog.Logger
	ghClient  *igithub.Client
	store     *store.Store
	processor piface.ProcessorAPI
	owner     string
	repo      string
	interval  time.Duration
}

func NewPoller(
	logger *slog.Logger,
	ghClient *igithub.Client,
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
