package webhook

import (
	"log/slog"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/marvin/miface"
	"github.com/mooneeb/amgi/internal/store"
)

type webhook struct {
	logger *slog.Logger
	secret string
	config *config.Config
	store  *store.Store
	marvin miface.MarvinAPI
}

func New(
	logger *slog.Logger,
	secret string,
	config *config.Config,
	store *store.Store,
	marvin miface.MarvinAPI,
) *webhook {
	return &webhook{
		logger: logger,
		secret: secret,
		config: config,
		store:  store,
		marvin: marvin,
	}
}
