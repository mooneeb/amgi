package polling

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/store"
)

// -----------------------------------------------------------------------------
// Fakes
// -----------------------------------------------------------------------------

// fakeGitHubClient implements GitHubClient with swappable funcs so each test
// controls what issues/PRs are returned (or what error surfaces). Call
// counters let us assert whether list methods were skipped on abort paths.
type fakeGitHubClient struct {
	listIssuesFunc       func(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error)
	listPullRequestsFunc func(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error)
	listIssuesCalls      int
	listPRsCalls         int
	lastSinceIssues      time.Time
	lastSincePRs         time.Time
	mu                   sync.Mutex // guards counters — Run() may be mid-tick on ctx cancel
}

func (f *fakeGitHubClient) ListIssues(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error) {
	f.mu.Lock()
	f.listIssuesCalls++
	f.lastSinceIssues = since
	f.mu.Unlock()
	if f.listIssuesFunc != nil {
		return f.listIssuesFunc(ctx, owner, repo, since)
	}
	return nil, nil
}

func (f *fakeGitHubClient) ListPullRequests(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error) {
	f.mu.Lock()
	f.listPRsCalls++
	f.lastSincePRs = since
	f.mu.Unlock()
	if f.listPullRequestsFunc != nil {
		return f.listPullRequestsFunc(ctx, owner, repo, since)
	}
	return nil, nil
}

// fakeProcessor implements piface.ProcessorAPI. Tick calls only Process();
// RetryPending isn't exercised from the polling path so it's a stub.
type fakeProcessor struct {
	processFunc     func(ctx context.Context, e *event.Event) error
	processCalls    int
	processedEvents []*event.Event
	mu              sync.Mutex
}

func (f *fakeProcessor) Process(ctx context.Context, e *event.Event) error {
	f.mu.Lock()
	f.processCalls++
	f.processedEvents = append(f.processedEvents, e)
	f.mu.Unlock()
	if f.processFunc != nil {
		return f.processFunc(ctx, e)
	}
	return nil
}

func (f *fakeProcessor) RetryPending(ctx context.Context) error { return nil }

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

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

// newTestEvent builds a minimal issue event (polling normalizes all listed
// items into *event.Event before handing them to the processor).
func newTestEvent(number int, title string) *event.Event {
	return &event.Event{
		Type:   string(event.EventTypeIssue),
		Owner:  "test-owner",
		Repo:   "test-repo",
		Number: number,
		Title:  title,
		Action: event.EventActionOpened,
	}
}

// newTestPullRequest builds a minimal pull_request event.
func newTestPullRequest(number int, title string) *event.Event {
	return &event.Event{
		Type:   string(event.EventTypePullRequest),
		Owner:  "test-owner",
		Repo:   "test-repo",
		Number: number,
		Title:  title,
		Action: event.EventActionOpened,
	}
}

func newTestPoller(t *testing.T, gh *fakeGitHubClient, proc *fakeProcessor) (*Poller, *store.Store) {
	t.Helper()
	s := newTestStore(t)
	p := NewPoller(
		discardLogger(),
		gh,
		s,
		proc,
		"test-owner",
		"test-repo",
		50*time.Millisecond, // short interval for Run() tests
	)
	return p, s
}

// -----------------------------------------------------------------------------
// TestTick_FirstPoll_NoCursor: on the very first tick, GetPollCursor reports
// not-found → `since` is defaulted to time.Now(); the poll still runs the
// list calls and writes a cursor row so the NEXT tick has a real cursor.
// -----------------------------------------------------------------------------

func TestTick_FirstPoll_NoCursor(t *testing.T) {
	gh := &fakeGitHubClient{}
	proc := &fakeProcessor{}
	p, s := newTestPoller(t, gh, proc)

	before := time.Now().UTC()
	if err := p.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	after := time.Now().UTC()

	// Lists must be called once each.
	if gh.listIssuesCalls != 1 || gh.listPRsCalls != 1 {
		t.Errorf("list calls: issues=%d prs=%d, want 1 each", gh.listIssuesCalls, gh.listPRsCalls)
	}

	// Cursor must now exist and sit within [before, after].
	cursor, found, err := s.GetPollCursor("test-owner", "test-repo")
	if err != nil {
		t.Fatalf("GetPollCursor: %v", err)
	}
	if !found {
		t.Fatal("cursor not written on first poll — subsequent polls would be incorrect")
	}
	// Allow equality on both bounds (clock could tick over a second boundary).
	// Store round-trips through RFC3339 (second precision) so use truncated bounds.
	if cursor.Before(before.Truncate(time.Second)) || cursor.After(after.Add(time.Second)) {
		t.Errorf("cursor = %v, want in [%v, %v]", cursor, before, after)
	}
}

// -----------------------------------------------------------------------------
// TestTick_SubsequentPoll_UsesStoredCursor: on the second tick, `since`
// comes from the stored cursor, and the cursor advances to ~now.
// -----------------------------------------------------------------------------

func TestTick_SubsequentPoll_UsesStoredCursor(t *testing.T) {
	gh := &fakeGitHubClient{}
	proc := &fakeProcessor{}
	p, s := newTestPoller(t, gh, proc)

	// Seed a cursor from 1 hour ago.
	seeded := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	if err := s.UpsertPollCursor("test-owner", "test-repo", seeded); err != nil {
		t.Fatalf("seed UpsertPollCursor: %v", err)
	}

	before := time.Now().UTC()
	if err := p.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	// Lists must have been called with `since` == the seeded cursor (not `now`).
	if !gh.lastSinceIssues.Equal(seeded) {
		t.Errorf("ListIssues since = %v, want %v (seeded cursor)", gh.lastSinceIssues, seeded)
	}
	if !gh.lastSincePRs.Equal(seeded) {
		t.Errorf("ListPullRequests since = %v, want %v (seeded cursor)", gh.lastSincePRs, seeded)
	}

	// Cursor must have advanced to ~now.
	cursor, found, err := s.GetPollCursor("test-owner", "test-repo")
	if err != nil || !found {
		t.Fatalf("GetPollCursor: err=%v found=%v", err, found)
	}
	if !cursor.After(seeded) {
		t.Errorf("cursor = %v, did not advance past seeded %v", cursor, seeded)
	}
	if cursor.Before(before.Truncate(time.Second)) {
		t.Errorf("cursor = %v, expected near now (≥ %v)", cursor, before)
	}
}

// -----------------------------------------------------------------------------
// TestTick_HappyPath_MixedIssuesAndPRs: lists return events; Process is
// called for each; cursor advances.
// -----------------------------------------------------------------------------

func TestTick_HappyPath_MixedIssuesAndPRs(t *testing.T) {
	issues := []*event.Event{
		newTestEvent(1, "issue one"),
		newTestEvent(2, "issue two"),
	}
	prs := []*event.Event{
		newTestPullRequest(10, "pr ten"),
	}
	gh := &fakeGitHubClient{
		listIssuesFunc: func(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error) {
			return issues, nil
		},
		listPullRequestsFunc: func(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error) {
			return prs, nil
		},
	}
	proc := &fakeProcessor{}
	p, s := newTestPoller(t, gh, proc)

	if err := p.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if proc.processCalls != 3 {
		t.Errorf("Process calls = %d, want 3 (2 issues + 1 PR)", proc.processCalls)
	}

	// Cursor must have been written.
	_, found, err := s.GetPollCursor("test-owner", "test-repo")
	if err != nil || !found {
		t.Fatalf("GetPollCursor: err=%v found=%v", err, found)
	}
}

// -----------------------------------------------------------------------------
// TestTick_ListIssuesError_AbortsAndDoesNotAdvanceCursor is the D-042
// contract: if fetching fails, the cursor must not move (so the next tick
// re-covers the same window) and no event processing runs.
// -----------------------------------------------------------------------------

func TestTick_ListIssuesError_AbortsAndDoesNotAdvanceCursor(t *testing.T) {
	seeded := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

	gh := &fakeGitHubClient{
		listIssuesFunc: func(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error) {
			return nil, errors.New("github unreachable")
		},
	}
	proc := &fakeProcessor{}
	p, s := newTestPoller(t, gh, proc)
	if err := s.UpsertPollCursor("test-owner", "test-repo", seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := p.tick(context.Background())
	if err == nil {
		t.Fatal("tick returned nil, want error")
	}

	// PR list must not have been attempted (sequential abort).
	if gh.listPRsCalls != 0 {
		t.Errorf("ListPullRequests calls = %d, want 0 (should abort before PR fetch)", gh.listPRsCalls)
	}

	// Processor must not have been called.
	if proc.processCalls != 0 {
		t.Errorf("Process calls = %d, want 0", proc.processCalls)
	}

	// Cursor must be unchanged (D-042).
	cursor, _, err := s.GetPollCursor("test-owner", "test-repo")
	if err != nil {
		t.Fatalf("GetPollCursor: %v", err)
	}
	if !cursor.Equal(seeded) {
		t.Errorf("cursor = %v, want %v (must not advance on fetch error — D-042)", cursor, seeded)
	}
}

// -----------------------------------------------------------------------------
// TestTick_ListPRsError_AbortsAndDoesNotAdvanceCursor: same D-042 contract,
// but the failure is on the PR fetch (after issues succeeded). Issues are
// already in memory but must NOT be processed.
// -----------------------------------------------------------------------------

func TestTick_ListPRsError_AbortsAndDoesNotAdvanceCursor(t *testing.T) {
	seeded := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

	gh := &fakeGitHubClient{
		listIssuesFunc: func(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error) {
			return []*event.Event{newTestEvent(1, "issue")}, nil
		},
		listPullRequestsFunc: func(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error) {
			return nil, errors.New("github unreachable")
		},
	}
	proc := &fakeProcessor{}
	p, s := newTestPoller(t, gh, proc)
	if err := s.UpsertPollCursor("test-owner", "test-repo", seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := p.tick(context.Background())
	if err == nil {
		t.Fatal("tick returned nil, want error")
	}

	if proc.processCalls != 0 {
		t.Errorf("Process calls = %d, want 0 (issues fetched but cannot process without successful full fetch)",
			proc.processCalls)
	}

	cursor, _, err := s.GetPollCursor("test-owner", "test-repo")
	if err != nil {
		t.Fatalf("GetPollCursor: %v", err)
	}
	if !cursor.Equal(seeded) {
		t.Errorf("cursor = %v, want %v (must not advance on fetch error — D-042)", cursor, seeded)
	}
}

// -----------------------------------------------------------------------------
// TestTick_PerEventProcessFailure_ContinuesAndAdvancesCursor is the D-041
// contract: a per-event Process() failure must NOT abort the tick and must
// NOT prevent the cursor from advancing. The rationale: the cursor tracks
// "last successful fetch window," not "all events perfectly processed" —
// processor handles its own retry lifecycle via PendingRetry.
// -----------------------------------------------------------------------------

func TestTick_PerEventProcessFailure_ContinuesAndAdvancesCursor(t *testing.T) {
	seeded := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

	issues := []*event.Event{
		newTestEvent(1, "doomed issue"),
		newTestEvent(2, "good issue"),
	}
	gh := &fakeGitHubClient{
		listIssuesFunc: func(ctx context.Context, owner, repo string, since time.Time) ([]*event.Event, error) {
			return issues, nil
		},
	}
	proc := &fakeProcessor{
		processFunc: func(ctx context.Context, e *event.Event) error {
			if e.Number == 1 {
				return errors.New("processor transient failure")
			}
			return nil
		},
	}
	p, s := newTestPoller(t, gh, proc)
	if err := s.UpsertPollCursor("test-owner", "test-repo", seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// tick itself MUST return nil — per-event failures are logged and swallowed.
	if err := p.tick(context.Background()); err != nil {
		t.Fatalf("tick returned %v, want nil (per-event failures must not abort tick — D-041)", err)
	}

	// Both issues must have been attempted (loop continued past failure).
	if proc.processCalls != 2 {
		t.Errorf("Process calls = %d, want 2 (loop must continue past per-event failure)",
			proc.processCalls)
	}

	// Cursor must have advanced (D-041).
	cursor, _, err := s.GetPollCursor("test-owner", "test-repo")
	if err != nil {
		t.Fatalf("GetPollCursor: %v", err)
	}
	if !cursor.After(seeded) {
		t.Errorf("cursor = %v, want advanced past %v (cursor must advance regardless of per-event outcomes — D-041)",
			cursor, seeded)
	}
}

// -----------------------------------------------------------------------------
// TestRun_ContextCancelReturnsCleanly: Run() must exit promptly when its
// context is canceled. Uses a short interval + immediate cancellation; the
// boot-time tick runs synchronously against the fake, then the ticker-loop
// select picks up ctx.Done().
// -----------------------------------------------------------------------------

func TestRun_ContextCancelReturnsCleanly(t *testing.T) {
	gh := &fakeGitHubClient{}
	proc := &fakeProcessor{}
	p, _ := newTestPoller(t, gh, proc)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- p.Run(ctx)
	}()

	// Give the boot-time tick a moment to run, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil on clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit within 2s after ctx cancel — select on ctx.Done() may be broken")
	}

	// Boot-time tick must have run at least once (proves the pre-ticker tick
	// fires and isn't gated on ctx.Done checking first).
	if gh.listIssuesCalls < 1 {
		t.Errorf("ListIssues calls = %d, want ≥ 1 (boot-time tick should run before ticker)",
			gh.listIssuesCalls)
	}
}
