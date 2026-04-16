package store

import (
	"os"
	"testing"

	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/logger"
)

func TestNew_CreatesTables(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	// Verify github_artifacts table exists
	row := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='github_artifacts'")
	var count int
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("github_artifacts table not created, got count=%d", count)
	}

	// Verify poll_state table exists
	row = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='poll_state'")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("poll_state table not created, got count=%d", count)
	}
}

func TestInsert(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	e := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 1,
		Type:   "test",
		Title:  "test",
	}
	err = s.Insert(&e, StoreStatusProcessed)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	row := s.db.QueryRow("SELECT count(*) FROM github_artifacts WHERE owner = ? AND repo = ? AND number = ?", e.Owner, e.Repo, e.Number)
	var count int
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("failed to scan github_artifacts: %v", err)
	}

	if count != 1 {
		t.Errorf("github_artifacts table not created, got count=%d, expected 1", count)
	}

}

func TestExists(t *testing.T) {
	tempdir := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tempdir)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	e := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 1,
		Type:   "test",
		Title:  "test",
	}
	err = s.Insert(&e, StoreStatusProcessed)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	p, err := s.HasEvent(e.Owner, e.Repo, e.Number)
	if err != nil {
		t.Fatalf("Exists() failed: %v", err)
	}

	if !p {
		t.Errorf("Exists() returned false, expected true")
	}

	// pending_retry should also return true — Exists checks any status
	e2 := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 2,
		Type:   "test",
		Title:  "test",
	}
	err = s.Insert(&e2, StoreStatusPendingRetry)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	p, err = s.HasEvent(e2.Owner, e2.Repo, e2.Number)
	if err != nil {
		t.Fatalf("Exists() failed: %v", err)
	}
	if !p {
		t.Errorf("Exists() returned false for pending_retry event, expected true")
	}
}

func TestIsProcessed(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	// processed event should return true
	e := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 1,
		Type:   "issue",
		Title:  "test",
	}
	err = s.Insert(&e, StoreStatusProcessed)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}
	p, err := s.IsEventProcessed(e.Owner, e.Repo, e.Number)
	if err != nil {
		t.Fatalf("IsProcessed() failed: %v", err)
	}
	if !p {
		t.Errorf("IsProcessed() returned false for processed event, expected true")
	}

	// pending_retry event should return false
	e2 := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 2,
		Type:   "issue",
		Title:  "test",
	}
	err = s.Insert(&e2, StoreStatusPendingRetry)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}
	p, err = s.IsEventProcessed(e2.Owner, e2.Repo, e2.Number)
	if err != nil {
		t.Fatalf("IsProcessed() failed: %v", err)
	}
	if p {
		t.Errorf("IsProcessed() returned true for pending_retry event, expected false")
	}
}

func TestExists_NotInDB(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	// Query for an event that was never inserted
	p, err := s.HasEvent("ghost-owner", "ghost-repo", 999)
	if err != nil {
		t.Fatalf("Exists() failed: %v", err)
	}
	if p {
		t.Errorf("Exists() returned true for non-existent event, expected false")
	}
}

func TestInsert_Duplicate(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	e := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 1,
		Type:   "issue",
		Title:  "test issue",
	}

	// First insert should succeed
	err = s.Insert(&e, StoreStatusProcessed)
	if err != nil {
		t.Fatalf("first Insert() failed: %v", err)
	}

	// Second insert with same (owner, repo, number) should not error
	err = s.Insert(&e, StoreStatusProcessed)
	if err != nil {
		t.Errorf("duplicate Insert() returned error: %v", err)
	}

	// Should still be exactly 1 row
	var count int
	err = s.db.QueryRow("SELECT count(*) FROM github_artifacts WHERE owner = ? AND repo = ? AND number = ?", e.Owner, e.Repo, e.Number).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after duplicate insert, got %d", count)
	}
}

func TestMarkAs(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	e := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 1,
		Type:   "issue",
		Title:  "test issue",
	}

	// Insert as pending_retry
	err = s.Insert(&e, StoreStatusPendingRetry)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	// Verify status is pending_retry
	var status string
	err = s.db.QueryRow("SELECT status FROM github_artifacts WHERE owner = ? AND repo = ? AND number = ?", e.Owner, e.Repo, e.Number).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}
	if status != string(StoreStatusPendingRetry) {
		t.Errorf("expected status %q before MarkAsProcessed, got %q", StoreStatusPendingRetry, status)
	}

	// Mark as processed
	err = s.MarkAs(e.Owner, e.Repo, e.Number, StoreStatusProcessed)
	if err != nil {
		t.Fatalf("MarkAs() failed: %v", err)
	}

	// Verify status changed to processed
	err = s.db.QueryRow("SELECT status FROM github_artifacts WHERE owner = ? AND repo = ? AND number = ?", e.Owner, e.Repo, e.Number).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query status after mark: %v", err)
	}
	if status != string(StoreStatusProcessed) {
		t.Errorf("expected status %q after MarkAsProcessed, got %q", StoreStatusProcessed, status)
	}
}

func TestIncrementRetryCount(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()

	e := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 1,
		Type:   "issue",
		Title:  "test issue",
	}
	err = s.Insert(&e, StoreStatusPendingRetry)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	err = s.IncrementRetryCount(e.Owner, e.Repo, e.Number)
	if err != nil {
		t.Fatalf("IncrementRetryCount() failed: %v", err)
	}

	var retryCount int
	err = s.db.QueryRow("SELECT retry_count FROM github_artifacts WHERE owner = ? AND repo = ? AND number = ?", e.Owner, e.Repo, e.Number).Scan(&retryCount)
	if err != nil {
		t.Fatalf("failed to query retry count: %v", err)
	}
	if retryCount != 1 {
		t.Errorf("expected retry count 1, got %d", retryCount)
	}
}

func TestGetPendingRetryEvents(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	os.Setenv("AMGI_DB_PATH", tmpFile)
	defer os.Unsetenv("AMGI_DB_PATH")

	s, err := New(logger.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer s.db.Close()
	e := event.Event{
		Owner:    "test",
		Repo:   "test",
		Number: 1,
		Type:   "issue",
		Title:  "test issue",
	}
	err = s.Insert(&e, StoreStatusPendingRetry)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	events, err := s.GetPendingRetryEvents(3)
	if err != nil {
		t.Fatalf("GetPendingRetryEvents() failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}
