package webhook

import "log/slog"

type webhook struct {
	logger *slog.Logger
	secret string
}

func New(logger *slog.Logger, secret string) *webhook {
	return &webhook{
		logger: logger,
		secret: secret,
	}
}
