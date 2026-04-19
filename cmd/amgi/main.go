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

// The main function is the entry point and serves as the
// Shepherd of AMGI. It is responsible for spinning up
// four worker services in Go Routines required for
// supporting Github Polling and Webhook and listening
// to SIGINT and SIGTERM to gracefully shutdown the server.
func main() {

	l := logger.New()
	m := os.Getenv("MARVIN_API_TOKEN")
	if m == "" {
		l.Error("MARVIN_API_TOKEN is not set")
		os.Exit(1)
	}

	c, err := loadConfig()
	if err != nil {
		l.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	l.Info("Config loaded successfully")

	store, err := store.New(l)
	if err != nil {
		l.Error("Failed to create store", "error", err)
		os.Exit(1)
	}

	l.Info("Store created successfully")

	marvin := marvin.New(l, &m, http.DefaultClient)
	if err != nil {
		l.Error("Failed to create Marvin client", "error", err)
		os.Exit(1)
	}

	l.Info("Marvin client created successfully")

	p := processor.New(l, c, store, marvin)

	l.Info("Processor created successfully")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	if hasWebhookModeConfigured(c) {
		s := os.Getenv("GITHUB_WEBHOOK_SECRET")
		if s == "" {
			l.Error("GITHUB_WEBHOOK_SECRET is not set")
			os.Exit(1)
		}
		wh := webhook.New(l, s, c, p)
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

		server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Info("Starting AMGI Webhook Server", "path", path, "port", port)
			err = server.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				l.Error("Failed to start AMGI Server", "error", err)
			}
		}()

		go func() {
			<-ctx.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			err := server.Shutdown(shutdownCtx)
			if err != nil {
				l.Error("Failed to shutdown AMGI Server", "error", err)
			}
		}()
	}

	if hasPollingModeConfigured(c) {
		ghToken := os.Getenv("GITHUB_TOKEN")
		if ghToken == "" {
			l.Error("GITHUB_TOKEN is not set")
			os.Exit(1)
		}
		ghClient := igithub.New(l, ghToken)

		for _, owner := range c.GitHub.Owners {
			itv, err := resolve.ResolvePollingInterval(&owner)
			if err != nil {
				l.Error("Failed to resolve polling interval", "error", err)
				continue
			}
			for _, repository := range owner.Repositories {
				if isPollingMode(&owner) {
					wg.Add(1)
					go func(ownerName string, repoName string, interval time.Duration) {
						defer wg.Done()
						poller := polling.NewPoller(
							l,
							ghClient,
							store,
							p,
							ownerName,
							repoName,
							interval,
						)
						err := poller.Run(ctx)
						if err != nil {
							l.Error("Failed to run poller", "error", err)
							return
						}
					}(owner.Name, repository.Name, itv)
				}
			}
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		// TODO: Make this configurable
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := p.RetryPending(ctx)
				if err != nil {
					l.Error("Failed to retry pending events", "error", err)
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
	l.Info("shut down sequence completed.")
}

func loadConfig() (*config.Config, error) {
	var cfgPath string

	if os.Getenv("CONFIG_PATH") != "" {
		cfgPath = os.Getenv("CONFIG_PATH")
	} else {
		cfgPath = "/etc/amgi/config.yaml"
	}

	c, err := validate.ParseAndValidateConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and validate config: %w", err)
	}

	return c, nil
}

func hasWebhookModeConfigured(cfg *config.Config) bool {
	for _, owner := range cfg.GitHub.Owners {
		if owner.Mode == config.ModeWebhook {
			return true
		}
	}
	return false
}

func hasPollingModeConfigured(cfg *config.Config) bool {
	for _, owner := range cfg.GitHub.Owners {
		if owner.Mode == config.ModePolling {
			return true
		}
	}
	return false
}

func isPollingMode(owner *config.Owner) bool {
	return owner.Mode == config.ModePolling
}
