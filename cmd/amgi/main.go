package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/config/resolve"
	"github.com/mooneeb/amgi/internal/config/validate"
	igithub "github.com/mooneeb/amgi/internal/github"
	"github.com/mooneeb/amgi/internal/github/polling"
	"github.com/mooneeb/amgi/internal/github/webhook"
	"github.com/mooneeb/amgi/internal/logger"
	"github.com/mooneeb/amgi/internal/marvin"
	"github.com/mooneeb/amgi/internal/processor"
	"github.com/mooneeb/amgi/internal/store"
)

// main is the AMGI entrypoint. It constructs shared dependencies
// (logger, store, Marvin client, processor), spawns per-worker
// goroutines (webhook server, pollers, retry sweep), and blocks
// on SIGINT/SIGTERM to trigger graceful shutdown.
func main() {
	l := logger.New()

	marvinToken := os.Getenv("MARVIN_API_TOKEN")
	if marvinToken == "" {
		l.Error("MARVIN_API_TOKEN is not set")
		os.Exit(1)
	}

	c, err := loadConfig()
	if err != nil {
		l.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	l.Info("config loaded successfully")

	// Validate all required env vars based on the resolved config shape
	// before doing any network I/O. Fail-fast so missing secrets don't
	// incur Marvin API calls for no reason.
	webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if hasModeConfigured(c, config.ModeWebhook) && webhookSecret == "" {
		l.Error("GITHUB_WEBHOOK_SECRET is not set (required for webhook mode)")
		os.Exit(1)
	}
	ghToken := os.Getenv("GITHUB_TOKEN")
	if hasModeConfigured(c, config.ModePolling) && ghToken == "" {
		l.Error("GITHUB_TOKEN is not set (required for polling mode)")
		os.Exit(1)
	}

	st, err := store.New(l)
	if err != nil {
		l.Error("failed to create store", "error", err)
		os.Exit(1)
	}
	l.Info("store created successfully")

	marvinHTTPClient := &http.Client{Timeout: 30 * time.Second}
	marvinClient := marvin.New(l, &marvinToken, marvinHTTPClient)
	l.Info("marvin client created successfully")

	// Fetch Marvin categories + labels and validate every list_name / label_names
	// reference in the config resolves to a real Marvin ID. Fail-fast at startup
	// so misspelled names never silently produce ghost-labeled tasks.
	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = marvinClient.Initialize(initCtx, c)
	initCancel()
	if err != nil {
		l.Error("failed to initialize marvin client", "error", err)
		os.Exit(1)
	}
	l.Info("marvin client initialized (categories + labels cached; config references validated)")

	proc := processor.New(l, c, st, marvinClient)
	l.Info("processor created successfully")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// Boot summary: surface defaults that are about to take effect so operators
	// can spot "match everything" or default-interval cases at startup instead
	// of discovering them via log volume later.
	for _, owner := range c.GitHub.Owners {
		for _, repository := range owner.Repositories {
			if resolve.ResolveFilters(c, &owner, &repository) == nil {
				l.Info("no filters configured, matching all events",
					"owner", owner.Name, "repo", repository.Name)
			}
		}
	}

	retryInterval := resolve.ResolveRetryInterval(c)
	retrySource := "default"
	if c.RetryIntervalSeconds != nil {
		retrySource = "config"
	}
	l.Info("resolved retry sweep interval", "interval", retryInterval, "source", retrySource)

	if hasModeConfigured(c, config.ModeWebhook) {
		wh := webhook.New(l, webhookSecret, c, proc)
		port := config.DefaultWebhookPort
		path := config.DefaultWebhookPath
		if c.WebhookServer != nil {
			if c.WebhookServer.Port != nil {
				port = *c.WebhookServer.Port
			}
			if c.WebhookServer.Path != nil {
				path = *c.WebhookServer.Path
			}
		}
		mux := http.NewServeMux()
		mux.HandleFunc(path, wh.Handler)

		server := &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           mux,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Info("starting webhook server", "path", path, "port", port)
			err := server.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				l.Error("webhook server error", "error", err)
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ctx.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			err := server.Shutdown(shutdownCtx)
			if err != nil {
				l.Error("webhook server shutdown failed", "error", err)
			}
		}()
	}

	if hasModeConfigured(c, config.ModePolling) {
		ghClient := igithub.New(l, ghToken)

		for _, owner := range c.GitHub.Owners {
			if !isPollingMode(&owner) {
				continue
			}
			itv := resolve.ResolvePollingInterval(&owner)
			pollSource := "default"
			if owner.PollingIntervalSeconds != nil {
				pollSource = "config"
			}
			l.Info("resolved polling interval", "owner", owner.Name, "interval", itv, "source", pollSource)
			for _, repository := range owner.Repositories {
				wg.Add(1)
				go func(ownerName string, repoName string, interval time.Duration) {
					defer wg.Done()
					poller := polling.NewPoller(
						l,
						ghClient,
						st,
						proc,
						ownerName,
						repoName,
						interval,
					)
					err := poller.Run(ctx)
					if err != nil {
						l.Error("poller run failed", "error", err)
						return
					}
				}(owner.Name, repository.Name, itv)
			}
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(retryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := proc.RetryPending(ctx)
				if err != nil {
					l.Error("retry sweep failed", "error", err)
				}
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	sig := <-sigCh
	l.Info("received signal, shutting down", "signal", sig)

	cancel()
	wg.Wait()
	l.Info("shutdown sequence completed")
}

func loadConfig() (*config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath
	}

	c, err := validate.ParseAndValidateConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and validate config: %w", err)
	}

	return c, nil
}

// hasModeConfigured reports whether at least one owner in the config uses the given mode.
func hasModeConfigured(cfg *config.Config, mode config.ModeType) bool {
	for _, owner := range cfg.GitHub.Owners {
		if owner.Mode == mode {
			return true
		}
	}
	return false
}

func isPollingMode(owner *config.Owner) bool {
	return owner.Mode == config.ModePolling
}
