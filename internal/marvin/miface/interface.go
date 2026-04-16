package miface

import (
	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

type MarvinAPI interface {
	AddTask(marvinConfig *config.MarvinConfig, event *event.Event) error
}
