package webhook

import (
	"log/slog"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/store"
)

type webhook struct {
	logger *slog.Logger
	secret string
	config *config.Config
	store  *store.Store
}

func New(
	logger *slog.Logger,
	secret string,
	config *config.Config,
	store *store.Store,
) *webhook {
	return &webhook{
		logger: logger,
		secret: secret,
		config: config,
		store:  store,
	}
}
