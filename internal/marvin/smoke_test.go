//go:build integration

// Integration / smoke tests that hit the real Marvin API.
// Excluded from default `go test` runs by the build tag above.
// Run manually:
//
//	MARVIN_API_TOKEN=<token> go test -tags integration -v -run Integration ./internal/marvin/
//
// These tests create a real task in your Marvin Inbox; delete them manually
// after the run. The test title is prefixed with AMGI-SMOKE-TEST for easy
// identification.

package marvin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

func getRealClient(t *testing.T) (*marvin, string) {
	t.Helper()
	token := os.Getenv("MARVIN_API_TOKEN")
	if token == "" {
		t.Skip("MARVIN_API_TOKEN not set; skipping integration test")
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	api := New(logger, &token, http.DefaultClient)
	m, ok := api.(*marvin)
	if !ok {
		t.Fatalf("expected *marvin, got %T", api)
	}
	return m, token
}

// TestIntegration_InitializeAgainstRealMarvin verifies the full Initialize
// flow (GET categories + GET labels + cache population + config validation)
// against the real Marvin API. Uses a config with a label_name ("mooneeb")
// we empirically confirmed exists in this account.
func TestIntegration_InitializeAgainstRealMarvin(t *testing.T) {
	m, _ := getRealClient(t)

	cfg := &config.Config{
		Marvin: config.Marvin{
			Configs: []config.MarvinConfig{
				{
					ID:         "smoke-cfg-inbox",
					LabelNames: []string{"mooneeb"},
					Task: config.MarvinTask{
						TitleTemplate: "unused for Initialize test",
						NoteTemplate:  "",
					},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.Initialize(ctx, cfg); err != nil {
		t.Fatalf("Initialize against real Marvin: %v", err)
	}

	m.cacheMu.RLock()
	catCount := len(m.categoriesCache)
	labelCount := len(m.labelsCache)
	m.cacheMu.RUnlock()

	t.Logf("Initialize OK: %d categories + %d labels cached", catCount, labelCount)
	if catCount == 0 {
		t.Error("expected at least 1 category in cache; got 0")
	}
	if labelCount == 0 {
		t.Error("expected at least 1 label in cache; got 0")
	}
}

// TestIntegration_AddTaskWithResolvedLabel creates a real task in the Marvin
// Inbox with the "mooneeb" label attached via name resolution. After running,
// verify visually in Marvin's UI that the task shows the "mooneeb" label
// (rendering correctly — not the ghost-label failure mode we saw earlier).
//
// This test leaves a task in your Marvin Inbox. Delete it manually when done.
// Titles are prefixed with AMGI-SMOKE-TEST and a Unix timestamp for easy search.
func TestIntegration_AddTaskWithResolvedLabel(t *testing.T) {
	m, _ := getRealClient(t)

	taskTitle := fmt.Sprintf("AMGI-SMOKE-TEST-%d", time.Now().Unix())
	cfg := &config.Config{
		Marvin: config.Marvin{
			Configs: []config.MarvinConfig{
				{
					ID:         "smoke-cfg-add",
					LabelNames: []string{"mooneeb"}, // resolver will look up the UUID
					Task: config.MarvinTask{
						TitleTemplate: taskTitle,
						NoteTemplate:  "Integration smoke test — AddTask with resolved label_name.",
					},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := m.Initialize(ctx, cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	testEvent := &event.Event{
		Owner:  "smoketest-owner",
		Repo:   "smoketest-repo",
		Number: 1,
		Type:   "issue",
		Title:  "Integration smoke test",
		Body:   "This is the body placeholder.",
	}

	if err := m.AddTask(ctx, &cfg.Marvin.Configs[0], testEvent); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	t.Logf("Task created with title %q and resolved label 'mooneeb'. Verify in Marvin UI; delete manually after.", taskTitle)
}
