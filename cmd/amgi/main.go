package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/config/validate"
	"github.com/mooneeb/amgi/internal/github/webhook"
	"github.com/mooneeb/amgi/internal/logger"
	"github.com/mooneeb/amgi/internal/marvin"
	"github.com/mooneeb/amgi/internal/store"
)

func main() {

	l := logger.New()
	s := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if s == "" {
		l.Error("GITHUB_WEBHOOK_SECRET is not set")
		os.Exit(1)
	}
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

	wh := webhook.New(l, s, c, store, marvin)

	l.Info("Webhook created successfully")

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

	http.HandleFunc(path, wh.Handler)

	l.Info("AMGI Server started", "path", path, "port", port)
	err = http.ListenAndServe(":"+strconv.Itoa(port), nil)
	if err != nil {
		l.Error("Failed to start AMGI Server", "error", err)
		os.Exit(1)
	}

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
