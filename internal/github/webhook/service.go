package webhook

import (
	"log/slog"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/processor/piface"
)

type webhook struct {
	logger    *slog.Logger
	secret    string
	config    *config.Config
	processor piface.ProcessorAPI
}

func New(
	logger *slog.Logger,
	secret string,
	config *config.Config,
	processor piface.ProcessorAPI,
) *webhook {
	return &webhook{
		logger:    logger,
		secret:    secret,
		config:    config,
		processor: processor,
	}
}
