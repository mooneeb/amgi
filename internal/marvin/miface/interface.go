package miface

import (
	"context"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

type MarvinAPI interface {
	AddTask(ctx context.Context, marvinConfig *config.MarvinConfig, event *event.Event) error
}
