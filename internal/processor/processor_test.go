package processor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/marvin"
	"github.com/mooneeb/amgi/internal/marvin/miface"
	"github.com/mooneeb/amgi/internal/store"
)

// -----------------------------------------------------------------------------
// Fake MarvinAPI
//
// Implements miface.MarvinAPI with a swappable AddTask function so each test
// controls the error-returning behavior. Call counters let tests assert
// whether AddTask was invoked (e.g. filter miss and duplicate cases must not
// call it).
// -----------------------------------------------------------------------------

type fakeMarvinAPI struct {
	addTaskFunc  func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error
	addTaskCalls int
	lastEvent    *event.Event
}

func (f *fakeMarvinAPI) Initialize(ctx context.Context, cfg *config.Config) error {
	return nil
}

func (f *fakeMarvinAPI) AddTask(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
	f.addTaskCalls++
	f.lastEvent = e
	if f.addTaskFunc != nil {
		return f.addTaskFunc(ctx, mc, e)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// newTestStore returns a fresh *store.Store backed by SQLite in a temp dir.
// AMGI_DB_PATH is the only way to point store.New at a non-default location,
// and t.Setenv scopes the override to this test.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "amgi.db")
	t.Setenv("AMGI_DB_PATH", dbPath)
	s, err := store.New(discardLogger())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s
}

// newTestConfig builds the minimal valid config for the processor path:
// one owner with one repo, one marvin config, NO filters at any level so
// every event matches. Callers mutate as needed for filter-miss scenarios.
func newTestConfig() *config.Config {
	return &config.Config{
		Version: "1",
		GitHub: config.GitHub{
			Owners: []config.Owner{
				{
					Name:           "test-owner",
					Mode:           config.ModeWebhook,
					MarvinConfigID: "default",
					Repositories: []config.Repository{
						{Name: "test-repo"},
					},
				},
			},
		},
		Marvin: config.Marvin{
			Configs: []config.MarvinConfig{
				{
					ID: "default",
					Task: config.MarvinTask{
						TitleTemplate: "{{.Title}}",
						NoteTemplate:  "{{.URL}}",
					},
				},
			},
		},
	}
}

// newTestEvent builds a minimal issue event that the default config will
// resolve and (in the filter-less default config) match.
func newTestEvent() *event.Event {
	return &event.Event{
		Type:   string(event.EventTypeIssue),
		Owner:  "test-owner",
		Repo:   "test-repo",
		Number: 1,
		Title:  "example issue",
		Body:   "body",
		State:  "open",
		Action: event.EventActionOpened,
		Author: "octocat",
		URL:    "https://github.com/test-owner/test-repo/issues/1",
	}
}

// newProcessor assembles the three collaborators the processor uses. Kept as
// a helper so tests stay focused on behavior, not wiring.
func newProcessor(t *testing.T, cfg *config.Config, marvinAPI miface.MarvinAPI) (*processor, *store.Store) {
	t.Helper()
	s := newTestStore(t)
	p := &processor{
		logger:    discardLogger(),
		cfg:       cfg,
		store:     s,
		marvinAPI: marvinAPI,
	}
	return p, s
}

// storeRow is a lightweight read-back of github_artifacts. Tests use this to
// assert status and retry_count after running the processor.
type storeRow struct {
	status     store.StoreStatus
	retryCount int
	exists     bool
}

func readRow(t *testing.T, s *store.Store, e *event.Event) storeRow {
	t.Helper()
	// We reach into the DB via the Store's public surface: HasEvent tells us
	// existence; for status/retry_count we need a direct query. store exports
	// no such method, so we use GetPendingRetryEvents to observe PendingRetry
	// rows and IsEventProcessed for Processed rows. For Failed rows, neither
	// helper exposes them directly, so we do a minimal SQL query below.
	exists, err := s.HasEvent(e.Owner, e.Repo, e.Number)
	if err != nil {
		t.Fatalf("HasEvent: %v", err)
	}
	if !exists {
		return storeRow{exists: false}
	}
	row := storeRow{exists: true}

	processed, err := s.IsEventProcessed(e.Owner, e.Repo, e.Number)
	if err != nil {
		t.Fatalf("IsEventProcessed: %v", err)
	}
	if processed {
		row.status = store.StoreStatusProcessed
		// retry_count for processed rows isn't load-bearing for our assertions.
		return row
	}

	// Check PendingRetry via GetPendingRetryEvents (threshold=MaxRetryAttempts+1
	// so we see rows at the cap too, for the 3-strike boundary).
	pending, err := s.GetPendingRetryEvents(config.MaxRetryAttempts + 1)
	if err != nil {
		t.Fatalf("GetPendingRetryEvents: %v", err)
	}
	for _, re := range pending {
		if re.Event.Owner == e.Owner && re.Event.Repo == e.Repo && re.Event.Number == e.Number {
			row.status = store.StoreStatusPendingRetry
			row.retryCount = re.RetryCount
			return row
		}
	}

	// Must be Failed (the only remaining status). We don't have a public helper,
	// so infer: exists=true, not processed, not pending-retry → Failed.
	row.status = store.StoreStatusFailed
	return row
}

// -----------------------------------------------------------------------------
// TestProcess covers the 9-branch dispatch in processor.Process:
//   - success → Processed
//   - DailyBudgetExceededError → PendingRetry (D-044 contract)
//   - permanent APIError (400/401/404) → Failed
//   - transient APIError → PendingRetry
//   - unknown/unclassified error → PendingRetry (D-045 regression guard)
//   - filter miss → no store write, no AddTask
//   - duplicate → no second AddTask, state unchanged
// -----------------------------------------------------------------------------

func TestProcess(t *testing.T) {
	type want struct {
		status       store.StoreStatus
		exists       bool
		addTaskCalls int
	}

	tests := []struct {
		name        string
		makeConfig  func() *config.Config
		addTaskFunc func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error
		want        want
	}{
		{
			name:       "happy path — AddTask succeeds, event stored as Processed",
			makeConfig: newTestConfig,
			addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
				return nil
			},
			want: want{status: store.StoreStatusProcessed, exists: true, addTaskCalls: 1},
		},
		{
			name:       "daily budget exceeded — stored as PendingRetry (D-044)",
			makeConfig: newTestConfig,
			addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
				return &marvin.DailyBudgetExceededError{
					ResetsAt: time.Now().UTC().Add(12 * time.Hour),
				}
			},
			want: want{status: store.StoreStatusPendingRetry, exists: true, addTaskCalls: 1},
		},
		{
			name:       "permanent APIError 400 — stored as Failed",
			makeConfig: newTestConfig,
			addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
				return &marvin.APIError{StatusCode: 400, Body: "bad request"}
			},
			want: want{status: store.StoreStatusFailed, exists: true, addTaskCalls: 1},
		},
		{
			name:       "permanent APIError 401 — stored as Failed",
			makeConfig: newTestConfig,
			addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
				return &marvin.APIError{StatusCode: 401, Body: "unauthorized"}
			},
			want: want{status: store.StoreStatusFailed, exists: true, addTaskCalls: 1},
		},
		{
			name:       "permanent APIError 404 — stored as Failed",
			makeConfig: newTestConfig,
			addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
				return &marvin.APIError{StatusCode: 404, Body: "not found"}
			},
			want: want{status: store.StoreStatusFailed, exists: true, addTaskCalls: 1},
		},
		{
			name:       "transient APIError 500 — stored as PendingRetry",
			makeConfig: newTestConfig,
			addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
				return &marvin.APIError{StatusCode: 500, Body: "internal error"}
			},
			want: want{status: store.StoreStatusPendingRetry, exists: true, addTaskCalls: 1},
		},
		{
			name:       "unknown error (D-045 regression guard) — stored as PendingRetry",
			makeConfig: newTestConfig,
			addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
				// Not *DailyBudgetExceededError and not *APIError — must fall
				// through to the default branch, not crash or drop the event.
				return errors.New("network hiccup — could be anything")
			},
			want: want{status: store.StoreStatusPendingRetry, exists: true, addTaskCalls: 1},
		},
		{
			name: "filter miss — no store write, no AddTask",
			makeConfig: func() *config.Config {
				cfg := newTestConfig()
				// Require a label the event doesn't have.
				cfg.GitHub.Owners[0].Filters = &config.Filters{
					Issues: &config.IssueFilters{
						Labels: &config.FieldOperators{In: []string{"must-have"}},
					},
				}
				return cfg
			},
			addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
				t.Fatalf("AddTask should not be called on filter miss")
				return nil
			},
			want: want{exists: false, addTaskCalls: 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeMarvinAPI{addTaskFunc: tc.addTaskFunc}
			p, s := newProcessor(t, tc.makeConfig(), fake)

			err := p.Process(context.Background(), newTestEvent())
			if err != nil {
				t.Fatalf("Process returned unexpected error: %v", err)
			}

			if fake.addTaskCalls != tc.want.addTaskCalls {
				t.Errorf("AddTask calls = %d, want %d", fake.addTaskCalls, tc.want.addTaskCalls)
			}

			row := readRow(t, s, newTestEvent())
			if row.exists != tc.want.exists {
				t.Errorf("row exists = %v, want %v", row.exists, tc.want.exists)
			}
			if tc.want.exists && row.status != tc.want.status {
				t.Errorf("row status = %q, want %q", row.status, tc.want.status)
			}
		})
	}
}

// TestProcess_Duplicate covers the idempotency branch: calling Process twice
// for the same owner/repo/number must not call AddTask the second time and
// must not change the stored status.
func TestProcess_Duplicate(t *testing.T) {
	fake := &fakeMarvinAPI{
		addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
			return nil
		},
	}
	p, s := newProcessor(t, newTestConfig(), fake)

	// First call — happy path, stored as Processed, AddTask called once.
	if err := p.Process(context.Background(), newTestEvent()); err != nil {
		t.Fatalf("first Process: %v", err)
	}
	if fake.addTaskCalls != 1 {
		t.Fatalf("first call AddTask calls = %d, want 1", fake.addTaskCalls)
	}

	// Second call with the same event — must short-circuit on isIdempotent.
	if err := p.Process(context.Background(), newTestEvent()); err != nil {
		t.Fatalf("second Process: %v", err)
	}
	if fake.addTaskCalls != 1 {
		t.Errorf("AddTask calls after duplicate = %d, want 1 (no second call)", fake.addTaskCalls)
	}

	row := readRow(t, s, newTestEvent())
	if row.status != store.StoreStatusProcessed {
		t.Errorf("row status after duplicate = %q, want %q", row.status, store.StoreStatusProcessed)
	}
}

// -----------------------------------------------------------------------------
// TestRetryPending_Empty: empty pending queue is a no-op (no AddTask calls,
// no error).
// -----------------------------------------------------------------------------

func TestRetryPending_Empty(t *testing.T) {
	fake := &fakeMarvinAPI{
		addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
			t.Fatalf("AddTask should not be called when queue is empty")
			return nil
		},
	}
	p, _ := newProcessor(t, newTestConfig(), fake)

	if err := p.RetryPending(context.Background()); err != nil {
		t.Fatalf("RetryPending on empty queue: %v", err)
	}
	if fake.addTaskCalls != 0 {
		t.Errorf("AddTask calls = %d, want 0", fake.addTaskCalls)
	}
}

// TestRetryPending_SuccessPromotesToProcessed: a PendingRetry row where
// AddTask succeeds on retry must flip to Processed, and retry_count must NOT
// be incremented on the success path.
func TestRetryPending_SuccessPromotesToProcessed(t *testing.T) {
	fake := &fakeMarvinAPI{
		addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
			return nil // success on retry
		},
	}
	p, s := newProcessor(t, newTestConfig(), fake)

	// Seed one PendingRetry row at retry_count=1.
	e := newTestEvent()
	if err := s.Insert(e, store.StoreStatusPendingRetry); err != nil {
		t.Fatalf("seed Insert: %v", err)
	}
	if err := s.IncrementRetryCount(e.Owner, e.Repo, e.Number); err != nil {
		t.Fatalf("seed IncrementRetryCount: %v", err)
	}

	if err := p.RetryPending(context.Background()); err != nil {
		t.Fatalf("RetryPending: %v", err)
	}
	if fake.addTaskCalls != 1 {
		t.Errorf("AddTask calls = %d, want 1", fake.addTaskCalls)
	}

	row := readRow(t, s, e)
	if row.status != store.StoreStatusProcessed {
		t.Errorf("row status = %q, want %q", row.status, store.StoreStatusProcessed)
	}
}

// TestRetryPending_BudgetSkipDoesNotIncrementRetryCount is the D-044 "no
// wasted attempts" contract. When Marvin's daily budget is exhausted, the
// retry sweep must SKIP the event (not count it as a failed attempt toward
// the 3-strike promotion). This is distinct from a transient API error.
func TestRetryPending_BudgetSkipDoesNotIncrementRetryCount(t *testing.T) {
	fake := &fakeMarvinAPI{
		addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
			return &marvin.DailyBudgetExceededError{
				ResetsAt: time.Now().UTC().Add(6 * time.Hour),
			}
		},
	}
	p, s := newProcessor(t, newTestConfig(), fake)

	// Seed a PendingRetry row at retry_count=0.
	e := newTestEvent()
	if err := s.Insert(e, store.StoreStatusPendingRetry); err != nil {
		t.Fatalf("seed Insert: %v", err)
	}

	if err := p.RetryPending(context.Background()); err != nil {
		t.Fatalf("RetryPending: %v", err)
	}

	row := readRow(t, s, e)
	if row.status != store.StoreStatusPendingRetry {
		t.Errorf("row status = %q, want %q (budget-skip must not promote)",
			row.status, store.StoreStatusPendingRetry)
	}
	if row.retryCount != 0 {
		t.Errorf("retry_count = %d, want 0 (budget-skip must NOT increment; D-044)",
			row.retryCount)
	}
}

// TestRetryPending_TransientErrorIncrementsAndStaysPendingRetry: a transient
// (non-budget) error below the 3-strike threshold must increment retry_count
// and keep the row in PendingRetry.
func TestRetryPending_TransientErrorIncrementsAndStaysPendingRetry(t *testing.T) {
	fake := &fakeMarvinAPI{
		addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
			return &marvin.APIError{StatusCode: 503, Body: "service unavailable"}
		},
	}
	p, s := newProcessor(t, newTestConfig(), fake)

	// Seed at retry_count=0 — after one failed retry, new count = 1 < 3, so
	// still PendingRetry.
	e := newTestEvent()
	if err := s.Insert(e, store.StoreStatusPendingRetry); err != nil {
		t.Fatalf("seed Insert: %v", err)
	}

	if err := p.RetryPending(context.Background()); err != nil {
		t.Fatalf("RetryPending: %v", err)
	}

	row := readRow(t, s, e)
	if row.status != store.StoreStatusPendingRetry {
		t.Errorf("row status = %q, want %q", row.status, store.StoreStatusPendingRetry)
	}
	if row.retryCount != 1 {
		t.Errorf("retry_count = %d, want 1", row.retryCount)
	}
}

// TestRetryPending_ThreeStrikePromotesToFailed: when retry_count+1 reaches
// MaxRetryAttempts, the row must flip to Failed. Seed at retry_count=2 so
// one more failure produces 3 ≥ MaxRetryAttempts (=3).
func TestRetryPending_ThreeStrikePromotesToFailed(t *testing.T) {
	fake := &fakeMarvinAPI{
		addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
			return &marvin.APIError{StatusCode: 500, Body: "still broken"}
		},
	}
	p, s := newProcessor(t, newTestConfig(), fake)

	e := newTestEvent()
	if err := s.Insert(e, store.StoreStatusPendingRetry); err != nil {
		t.Fatalf("seed Insert: %v", err)
	}
	// Bump retry_count to 2 so the next failure is the third strike.
	for i := 0; i < 2; i++ {
		if err := s.IncrementRetryCount(e.Owner, e.Repo, e.Number); err != nil {
			t.Fatalf("seed IncrementRetryCount #%d: %v", i+1, err)
		}
	}

	if err := p.RetryPending(context.Background()); err != nil {
		t.Fatalf("RetryPending: %v", err)
	}

	row := readRow(t, s, e)
	if row.status != store.StoreStatusFailed {
		t.Errorf("row status = %q, want %q (three-strike should promote)",
			row.status, store.StoreStatusFailed)
	}
}

// TestRetryPending_ContinuesAfterPerEventFailure: when two pending events are
// processed, a failure on the first must not abort the second. Both rows
// should reach their expected end states.
func TestRetryPending_ContinuesAfterPerEventFailure(t *testing.T) {
	// The fake returns an error for issue #1 and succeeds for issue #2, so
	// we can assert that the loop continues past the first per-event failure.
	fake := &fakeMarvinAPI{
		addTaskFunc: func(ctx context.Context, mc *config.MarvinConfig, e *event.Event) error {
			if e.Number == 1 {
				return &marvin.APIError{StatusCode: 500, Body: "first event failed"}
			}
			return nil
		},
	}
	p, s := newProcessor(t, newTestConfig(), fake)

	e1 := newTestEvent() // Number = 1
	e2 := newTestEvent()
	e2.Number = 2
	e2.Title = "second issue"

	if err := s.Insert(e1, store.StoreStatusPendingRetry); err != nil {
		t.Fatalf("seed Insert e1: %v", err)
	}
	if err := s.Insert(e2, store.StoreStatusPendingRetry); err != nil {
		t.Fatalf("seed Insert e2: %v", err)
	}

	if err := p.RetryPending(context.Background()); err != nil {
		t.Fatalf("RetryPending: %v", err)
	}

	if fake.addTaskCalls != 2 {
		t.Errorf("AddTask calls = %d, want 2 (both events must be attempted)",
			fake.addTaskCalls)
	}

	row1 := readRow(t, s, e1)
	if row1.status != store.StoreStatusPendingRetry {
		t.Errorf("e1 status = %q, want %q (transient failure, still pending)",
			row1.status, store.StoreStatusPendingRetry)
	}
	if row1.retryCount != 1 {
		t.Errorf("e1 retry_count = %d, want 1", row1.retryCount)
	}

	row2 := readRow(t, s, e2)
	if row2.status != store.StoreStatusProcessed {
		t.Errorf("e2 status = %q, want %q (should have succeeded despite e1 failure)",
			row2.status, store.StoreStatusProcessed)
	}
}
