package processor

import (
	"log/slog"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/marvin/miface"
	"github.com/mooneeb/amgi/internal/processor/piface"
	"github.com/mooneeb/amgi/internal/store"
)

type processor struct {
	logger    *slog.Logger
	cfg       *config.Config
	store     *store.Store
	marvinAPI miface.MarvinAPI
}

func New(
	logger *slog.Logger,
	cfg *config.Config,
	store *store.Store,
	marvinAPI miface.MarvinAPI,
) piface.ProcessorAPI {
	return &processor{logger: logger, cfg: cfg, store: store, marvinAPI: marvinAPI}
}
