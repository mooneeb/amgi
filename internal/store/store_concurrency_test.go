package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mooneeb/amgi/internal/logger"
)

// TestUpsertPollCursor_ConcurrentWritersDoNotErr exercises the concurrent-write
// pattern that surfaces in production: many goroutines (one per repository)
// upserting their poll cursor at the same time.
//
// On a store opened with SQLite defaults (journal_mode=DELETE, busy_timeout=0),
// most goroutines lose the writer-lock race and return SQLITE_BUSY. With the
// store correctly configured (WAL + busy_timeout via DSN), all goroutines
// either acquire the lock immediately or wait briefly, and every upsert
// succeeds.
//
// All goroutines fire simultaneously via a start-barrier channel to maximize
// contention. The test asserts (a) no goroutine returned any error, and (b)
// the final database state contains exactly one row per (owner, repo) pair
// with a non-zero cursor — proving both no-error AND no-data-loss.
func TestUpsertPollCursor_ConcurrentWritersDoNotErr(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	const N = 25
	start := make(chan struct{})
	errs := make([]error, N)
	var wg sync.WaitGroup
	wg.Add(N)

	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			<-start // wait at barrier
			owner := fmt.Sprintf("owner-%d", i)
			repo := fmt.Sprintf("repo-%d", i)
			errs[i] = s.UpsertPollCursor(owner, repo, time.Now().UTC())
		}(i)
	}

	// Release all goroutines simultaneously.
	close(start)
	wg.Wait()

	// Assertion 1: zero errors from any goroutine.
	for i, e := range errs {
		if e != nil {
			t.Errorf("goroutine %d: UpsertPollCursor failed: %v", i, e)
		}
	}

	// Assertion 2: exactly N rows present in poll_state.
	var count int
	err = s.db.QueryRow("SELECT count(*) FROM poll_state").Scan(&count)
	if err != nil {
		t.Fatalf("count poll_state: %v", err)
	}
	if count != N {
		t.Errorf("poll_state row count = %d, want %d", count, N)
	}

	// Assertion 3: every (owner, repo) pair retrievable with a non-zero cursor.
	for i := 0; i < N; i++ {
		owner := fmt.Sprintf("owner-%d", i)
		repo := fmt.Sprintf("repo-%d", i)
		cursor, found, err := s.GetPollCursor(owner, repo)
		if err != nil {
			t.Errorf("GetPollCursor(%s, %s) failed: %v", owner, repo, err)
			continue
		}
		if !found {
			t.Errorf("GetPollCursor(%s, %s): not found", owner, repo)
			continue
		}
		if cursor.IsZero() {
			t.Errorf("GetPollCursor(%s, %s): cursor is zero", owner, repo)
		}
	}
}

// TestUpsertPollCursor_WaitsForLock deterministically proves that
// busy_timeout is applied to the connection the store uses — i.e. the
// configured pragma actually takes effect, not just at DB-init time but on
// every connection the *sql.DB pool hands out.
//
// Setup: a separate *sql.DB pins a connection and holds the SQLite writer lock
// via "BEGIN IMMEDIATE" on that pinned connection. A goroutine schedules
// release of the lock after 500ms via "ROLLBACK" on the same pinned conn.
// Meanwhile, the store's UpsertPollCursor is invoked. It must wait for the
// lock (returning SUCCESSFULLY after the holder releases) rather than fail
// fast with SQLITE_BUSY (default behavior with busy_timeout=0).
//
// We use sql.Conn pinning + raw BEGIN IMMEDIATE rather than db.BeginTx
// because BeginTx in modernc.org/sqlite defaults to a deferred transaction
// unless the DSN sets _txlock=immediate. Explicit pinning + raw statement
// removes that footgun and makes the lock-hold semantics unambiguous.
//
// On main (busy_timeout=0): the store's call returns SQLITE_BUSY immediately
// → test fails. With the fix (busy_timeout=5000): the call waits ~500ms for
// the holder to rollback, then succeeds → test passes.
func TestUpsertPollCursor_WaitsForLock(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	// Seed an upsert so the store's pool has at least one initialized
	// connection in steady state. Removes any first-connection-open
	// ambiguity from the lock-hold timing below.
	err = s.UpsertPollCursor("seed-owner", "seed-repo", time.Now().UTC())
	if err != nil {
		t.Fatalf("seed UpsertPollCursor failed: %v", err)
	}

	ctx := context.Background()

	// Separate *sql.DB pointing at the same database file. Independent
	// connection pool so we can hold a lock without interfering with the
	// store's pool.
	holderDB, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("open holder DB: %v", err)
	}
	defer holderDB.Close()

	// Pin a single connection so the lock-hold and release both happen on
	// the SAME underlying SQLite connection (otherwise rollback may target
	// a different connection that doesn't hold the lock).
	holderConn, err := holderDB.Conn(ctx)
	if err != nil {
		t.Fatalf("pin holder conn: %v", err)
	}

	// Acquire the writer lock via explicit BEGIN IMMEDIATE. The IMMEDIATE
	// keyword acquires the RESERVED lock at BEGIN time (not lazily at first
	// write), guaranteeing the store's upsert below contends from the
	// moment it runs.
	_, err = holderConn.ExecContext(ctx, "BEGIN IMMEDIATE")
	if err != nil {
		t.Fatalf("BEGIN IMMEDIATE on holder: %v", err)
	}

	// Schedule release after holdDuration on the SAME pinned connection.
	const holdDuration = 500 * time.Millisecond
	go func() {
		time.Sleep(holdDuration)
		_, _ = holderConn.ExecContext(ctx, "ROLLBACK")
		_ = holderConn.Close()
	}()

	// Now run the store upsert. Without busy_timeout it returns immediately
	// with SQLITE_BUSY. With busy_timeout=5000 it waits for the rollback
	// and then succeeds.
	startTime := time.Now()
	err = s.UpsertPollCursor("test-owner", "test-repo", time.Now().UTC())
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("UpsertPollCursor returned error (likely SQLITE_BUSY without fix): %v", err)
	}

	// Must have waited for the lock to release. If elapsed is significantly
	// less than holdDuration, the lock-hold pattern broke (e.g. holder didn't
	// actually acquire the lock), which would render the test meaningless.
	if elapsed < holdDuration {
		t.Errorf("UpsertPollCursor returned in %v, expected >= %v (proves it waited for the lock; otherwise the holder may not have acquired RESERVED)", elapsed, holdDuration)
	}
}

// TestStore_PragmasApplied verifies the SQLite pragmas configured in store
// initialization (journal_mode=WAL, busy_timeout=5000) are actually applied
// on every connection the pool hands out — not just the first one.
//
// We hold three sql.Conn instances simultaneously so the pool must allocate
// distinct underlying SQLite connections. Querying PRAGMA on each proves the
// DSN-level _pragma parameters take effect on every newly-opened connection.
//
// This is a regression test for the future case where someone "simplifies"
// the store init by switching to `db.Exec("PRAGMA ...")` — which would only
// configure one pool connection and let the bug return silently.
func TestStore_PragmasApplied(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	ctx := context.Background()
	const N = 3
	conns := make([]*sql.Conn, N)
	for i := 0; i < N; i++ {
		c, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn(%d): %v", i, err)
		}
		defer c.Close()
		conns[i] = c
	}

	for i, c := range conns {
		var journalMode string
		if err := c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
			t.Fatalf("conn %d: PRAGMA journal_mode: %v", i, err)
		}
		if strings.ToLower(journalMode) != "wal" {
			t.Errorf("conn %d: journal_mode = %q, want %q", i, journalMode, "wal")
		}

		var busyTimeout int
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatalf("conn %d: PRAGMA busy_timeout: %v", i, err)
		}
		if busyTimeout != 5000 {
			t.Errorf("conn %d: busy_timeout = %d, want 5000", i, busyTimeout)
		}
	}
}
