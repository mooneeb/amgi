package marvin

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/mooneeb/amgi/internal/marvin/miface"
	"golang.org/x/time/rate"
)

type marvin struct {
	logger          *slog.Logger
	apiToken        *string
	baseURL         string
	client          *http.Client
	perSecond       *rate.Limiter
	reads           *rate.Limiter
	cacheMu         sync.RWMutex
	categoriesCache map[string]cacheEntry
	labelsCache     map[string]cacheEntry
	dailyMu         sync.Mutex
	dailyMax        int
	dailyCount      int
	dailyResetAt    time.Time
}

func New(
	logger *slog.Logger,
	apiToken *string,
	client *http.Client,
) miface.MarvinAPI {
	return &marvin{
		logger:       logger,
		apiToken:     apiToken,
		baseURL:      baseURL,
		client:       client,
		perSecond:    rate.NewLimiter(rate.Every(time.Second), 1),
		reads:        rate.NewLimiter(rate.Every(3*time.Second), 1),
		dailyMax:     defaultDailyMax,
		dailyResetAt: nextUTCMidnight(time.Now().UTC()),
	}
}
