package marvin

import (
	"log/slog"
	"net/http"

	"github.com/mooneeb/amgi/internal/marvin/miface"
)

type marvin struct {
	logger   *slog.Logger
	apiToken *string
	baseURL  string
	client   *http.Client
}

func New(
	logger *slog.Logger,
	apiToken *string,
	client *http.Client,
) miface.MarvinAPI {
	return &marvin{
		logger:   logger,
		apiToken: apiToken,
		baseURL:  baseURL,
		client:   client,
	}
}
