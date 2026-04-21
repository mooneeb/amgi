package marvin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mooneeb/amgi/internal/config"
	"golang.org/x/time/rate"
)

// discardLogger returns a slog logger that discards all output. Keeps test
// runner stdout clean of info/warn log lines unless the test is actively
// asserting on logs.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// testMarvin constructs a marvin struct pointed at a test server with rate
// limiters disabled (rate.Inf) so tests don't wait the 1-req-per-3-sec reads
// limit or 1-req-per-sec addTask limit.
func testMarvin(t *testing.T, baseURL string) *marvin {
	t.Helper()
	token := "test-token"
	return &marvin{
		logger:       discardLogger(),
		apiToken:     &token,
		baseURL:      baseURL,
		client:       http.DefaultClient,
		perSecond:    rate.NewLimiter(rate.Inf, 1),
		reads:        rate.NewLimiter(rate.Inf, 1),
		dailyMax:     1440,
		dailyResetAt: nextUTCMidnight(time.Now().UTC()),
	}
}

// fakeMarvin serves /api/categories and /api/labels from in-memory slices.
// Hit counters let tests assert on refresh behavior (how many times the
// endpoints were called).
type fakeMarvin struct {
	mu         sync.Mutex
	categories []category
	labels     []label
	catHits    int
	labelHits  int
}

func (f *fakeMarvin) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/categories":
			f.catHits++
			_ = json.NewEncoder(w).Encode(f.categories)
		case "/api/labels":
			f.labelHits++
			_ = json.NewEncoder(w).Encode(f.labels)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (f *fakeMarvin) categoryHits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.catHits
}

func (f *fakeMarvin) labelHitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.labelHits
}

// -----------------------------------------------------------------------------
// resolveList
// -----------------------------------------------------------------------------

func TestResolveList_ExactMatch(t *testing.T) {
	fake := &fakeMarvin{
		categories: []category{
			{ID: "id-work", Title: "Work", ParentID: "root"},
			{ID: "id-personal", Title: "Personal", ParentID: "root"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	got, err := m.resolveList(context.Background(), "Work")
	if err != nil {
		t.Fatalf("resolveList: %v", err)
	}
	if got != "id-work" {
		t.Errorf("got %q, want %q", got, "id-work")
	}
}

func TestResolveList_CaseInsensitive(t *testing.T) {
	fake := &fakeMarvin{
		categories: []category{
			{ID: "id-work", Title: "Work"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	cases := []string{"work", "WORK", "WoRk", "Work"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			got, err := m.resolveList(context.Background(), c)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != "id-work" {
				t.Errorf("got %q, want %q", got, "id-work")
			}
		})
	}
}

func TestResolveList_EmptyNameReturnsEmpty(t *testing.T) {
	// An empty list_name must return empty ID and no error so the caller can
	// omit parentId from the addTask body (→ Marvin's default Inbox behavior).
	m := testMarvin(t, "http://unused-this-test-does-not-call-http")
	got, err := m.resolveList(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestResolveList_CacheMissTriggersRefresh(t *testing.T) {
	// Scenario: populate cache from a server with only "Work". Then add
	// "Finance" to the server side. resolveList("Finance") should trigger
	// exactly one refresh and succeed.
	fake := &fakeMarvin{
		categories: []category{
			{ID: "id-work", Title: "Work"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	hitsBefore := fake.categoryHits()

	// Simulate a category added to Marvin after startup.
	fake.mu.Lock()
	fake.categories = append(fake.categories, category{ID: "id-finance", Title: "Finance"})
	fake.mu.Unlock()

	got, err := m.resolveList(context.Background(), "Finance")
	if err != nil {
		t.Fatalf("resolveList: %v", err)
	}
	if got != "id-finance" {
		t.Errorf("got %q, want %q", got, "id-finance")
	}

	hitsAfter := fake.categoryHits()
	if delta := hitsAfter - hitsBefore; delta != 1 {
		t.Errorf("expected exactly one refresh; got %d extra /api/categories hits", delta)
	}
}

func TestResolveList_NotFoundAfterRefresh(t *testing.T) {
	fake := &fakeMarvin{
		categories: []category{
			{ID: "id-work", Title: "Work"},
			{ID: "id-personal", Title: "Personal"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	_, err := m.resolveList(context.Background(), "Nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var notFound *ListNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ListNotFoundError, got %T: %v", err, err)
	}
	if notFound.Name != "Nonexistent" {
		t.Errorf("got name %q, want %q", notFound.Name, "Nonexistent")
	}
	if len(notFound.Available) != 2 {
		t.Errorf("got %d available titles, want 2", len(notFound.Available))
	}
	// Available should contain original-case titles, not lowercase keys.
	if !slices.Contains(notFound.Available, "Work") || !slices.Contains(notFound.Available, "Personal") {
		t.Errorf("Available should contain original-case titles; got %v", notFound.Available)
	}
}

// -----------------------------------------------------------------------------
// resolveLabels
// -----------------------------------------------------------------------------

func TestResolveLabels_AllHit(t *testing.T) {
	fake := &fakeMarvin{
		labels: []label{
			{ID: "id-bug", Title: "bug"},
			{ID: "id-work", Title: "work"},
			{ID: "id-github", Title: "github"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	got, err := m.resolveLabels(context.Background(), []string{"bug", "github"})
	if err != nil {
		t.Fatalf("resolveLabels: %v", err)
	}
	want := []string{"id-bug", "id-github"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveLabels_CaseInsensitive(t *testing.T) {
	fake := &fakeMarvin{
		labels: []label{
			{ID: "id-bug", Title: "Bug"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	got, err := m.resolveLabels(context.Background(), []string{"bug", "BUG", "Bug"})
	if err != nil {
		t.Fatalf("resolveLabels: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d ids, want 3", len(got))
	}
	for i, id := range got {
		if id != "id-bug" {
			t.Errorf("ids[%d]=%q, want id-bug", i, id)
		}
	}
}

func TestResolveLabels_EmptySliceReturnsNil(t *testing.T) {
	m := testMarvin(t, "http://unused")
	got, err := m.resolveLabels(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestResolveLabels_EmptyStringInSliceSkipped(t *testing.T) {
	fake := &fakeMarvin{
		labels: []label{
			{ID: "id-bug", Title: "bug"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	// Empty strings in the names slice should be silently skipped.
	got, err := m.resolveLabels(context.Background(), []string{"", "bug", ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(got, []string{"id-bug"}) {
		t.Errorf("got %v, want [id-bug]", got)
	}
}

func TestResolveLabels_OnMissRefreshesExactlyOnce(t *testing.T) {
	// All three requested labels are missing from the cache. We should see
	// exactly ONE refresh call, not three.
	fake := &fakeMarvin{
		labels: []label{
			{ID: "id-a", Title: "a"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	hitsBefore := fake.labelHitCount()

	_, err := m.resolveLabels(context.Background(), []string{"missing1", "missing2", "missing3"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var notFound *LabelNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected LabelNotFoundError, got %T: %v", err, err)
	}

	hitsAfter := fake.labelHitCount()
	if delta := hitsAfter - hitsBefore; delta != 1 {
		t.Errorf("expected exactly 1 refresh hit; got %d", delta)
	}
}

func TestResolveLabels_ReturnsFirstMissingName(t *testing.T) {
	// When multiple names are missing, the error reports the first one found
	// to be missing. Available lists all cached titles.
	fake := &fakeMarvin{
		labels: []label{
			{ID: "id-a", Title: "alpha"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	_, err := m.resolveLabels(context.Background(), []string{"alpha", "beta", "gamma"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var notFound *LabelNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected LabelNotFoundError, got %T", err)
	}
	if notFound.Name != "beta" {
		t.Errorf("got Name=%q, want beta (the first missing)", notFound.Name)
	}
}

// -----------------------------------------------------------------------------
// dedup on refresh
// -----------------------------------------------------------------------------

func TestRefresh_DeduplicatesDuplicateTitles(t *testing.T) {
	// Marvin isn't supposed to allow duplicate label titles, but if it did
	// (or returns duplicates due to race), the resolver should log + pick first.
	fake := &fakeMarvin{
		labels: []label{
			{ID: "id-first", Title: "duplicate"},
			{ID: "id-second", Title: "duplicate"},
			{ID: "id-unique", Title: "unique"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	if err := m.populateCaches(context.Background()); err != nil {
		t.Fatalf("populateCaches: %v", err)
	}

	got, err := m.resolveLabels(context.Background(), []string{"duplicate"})
	if err != nil {
		t.Fatalf("resolveLabels: %v", err)
	}
	if !slices.Equal(got, []string{"id-first"}) {
		t.Errorf("got %v, want [id-first] (first wins on dedup)", got)
	}

	// Verify the non-duplicate was cached correctly.
	got2, err := m.resolveLabels(context.Background(), []string{"unique"})
	if err != nil {
		t.Fatalf("resolveLabels unique: %v", err)
	}
	if !slices.Equal(got2, []string{"id-unique"}) {
		t.Errorf("got %v, want [id-unique]", got2)
	}
}

// -----------------------------------------------------------------------------
// Initialize (config-walk integration)
// -----------------------------------------------------------------------------

func TestInitialize_HappyPath(t *testing.T) {
	fake := &fakeMarvin{
		categories: []category{
			{ID: "id-work", Title: "Work"},
			{ID: "id-personal", Title: "Personal"},
		},
		labels: []label{
			{ID: "id-github", Title: "github"},
			{ID: "id-priority", Title: "priority"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	cfg := &config.Config{
		Marvin: config.Marvin{
			Configs: []config.MarvinConfig{
				{ID: "cfg1", ListName: "Work", LabelNames: []string{"github"}},
				{ID: "cfg2", ListName: "Personal", LabelNames: []string{"github", "priority"}},
			},
		},
	}

	if err := m.Initialize(context.Background(), cfg); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
}

func TestInitialize_OmittedListNameIsInboxFallback(t *testing.T) {
	// A config with no list_name (empty string) should validate successfully —
	// the task will land in Inbox because parentId will be omitted from addTask.
	fake := &fakeMarvin{
		categories: []category{
			{ID: "id-work", Title: "Work"},
		},
		labels: []label{
			{ID: "id-github", Title: "github"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	cfg := &config.Config{
		Marvin: config.Marvin{
			Configs: []config.MarvinConfig{
				{ID: "cfg1", ListName: "", LabelNames: []string{"github"}},
			},
		},
	}

	if err := m.Initialize(context.Background(), cfg); err != nil {
		t.Fatalf("Initialize with omitted list_name should succeed: %v", err)
	}
}

func TestInitialize_FailsOnUnresolvableListName(t *testing.T) {
	fake := &fakeMarvin{
		categories: []category{
			{ID: "id-work", Title: "Work"},
		},
		labels: []label{
			{ID: "id-github", Title: "github"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	cfg := &config.Config{
		Marvin: config.Marvin{
			Configs: []config.MarvinConfig{
				{ID: "cfg1", ListName: "Work", LabelNames: []string{"github"}},
				{ID: "cfg2", ListName: "Nonexistent", LabelNames: []string{"github"}},
			},
		},
	}

	err := m.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for cfg2's unresolvable list_name")
	}
	if !strings.Contains(err.Error(), `marvin config "cfg2"`) {
		t.Errorf("error should identify the failing config id; got: %v", err)
	}
	var listNotFound *ListNotFoundError
	if !errors.As(err, &listNotFound) {
		t.Errorf("expected wrapped ListNotFoundError, got %T: %v", err, err)
	}
}

func TestInitialize_FailsOnUnresolvableLabelName(t *testing.T) {
	fake := &fakeMarvin{
		categories: []category{
			{ID: "id-work", Title: "Work"},
		},
		labels: []label{
			{ID: "id-github", Title: "github"},
		},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	m := testMarvin(t, srv.URL)
	cfg := &config.Config{
		Marvin: config.Marvin{
			Configs: []config.MarvinConfig{
				{ID: "cfg1", ListName: "Work", LabelNames: []string{"github", "nonexistent"}},
			},
		},
	}

	err := m.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for unresolvable label_name")
	}
	if !strings.Contains(err.Error(), `marvin config "cfg1"`) {
		t.Errorf("error should identify the failing config id; got: %v", err)
	}
	var labelNotFound *LabelNotFoundError
	if !errors.As(err, &labelNotFound) {
		t.Errorf("expected wrapped LabelNotFoundError, got %T: %v", err, err)
	}
}
