package miface

import (
	"context"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

type MarvinAPI interface {
	Initialize(ctx context.Context, cfg *config.Config) error
	AddTask(ctx context.Context, marvinConfig *config.MarvinConfig, event *event.Event) error
}
