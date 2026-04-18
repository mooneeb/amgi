package piface

import (
	"context"

	"github.com/mooneeb/amgi/internal/event"
)

type ProcessorAPI interface {
	Process(ctx context.Context, e *event.Event) error
	RetryPending(ctx context.Context) error
}
