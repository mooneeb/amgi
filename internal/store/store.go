package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
	_ "modernc.org/sqlite"
)

type Store struct {
	db  *sql.DB
	log *slog.Logger
}

type StoreStatus string

const (
	StoreStatusProcessed    StoreStatus = "processed"
	StoreStatusPendingRetry StoreStatus = "pending_retry"
	StoreStatusFailed       StoreStatus = "failed"
)

func New(log *slog.Logger) (*Store, error) {
	path := os.Getenv("AMGI_DB_PATH")
	if path == "" {
		path = config.DefaultDBPath
	}
	// Build DSN with pragmas applied to every connection the pool opens.
	//
	//   busy_timeout=5000   Writers wait up to 5s for the lock instead of failing
	//                       fast (default is 0 — return SQLITE_BUSY immediately).
	//                       Required for the goroutine-per-repo poller pattern
	//                       where many writes contend on each poll tick.
	//   journal_mode=WAL    Improves reader/writer concurrency — readers no longer
	//                       block on writers. Writers still serialize; busy_timeout
	//                       handles writer/writer waits.
	//
	// modernc.org/sqlite supports DSN-level _pragma parameters that apply to every
	// newly-opened connection, avoiding the pool-init trap of `db.Exec("PRAGMA ...")`
	// only configuring one connection.
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	err = createTables(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return &Store{db: db, log: log}, nil

}

func createTables(db *sql.DB) error {
	err := createGithubArtifactsTable(db)
	if err != nil {
		return fmt.Errorf("failed to create github_artifacts table: %w", err)
	}
	err = createPollStateTable(db)
	if err != nil {
		return fmt.Errorf("failed to create poll_state table: %w", err)
	}
	return nil
}

func createGithubArtifactsTable(db *sql.DB) error {
	_, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS github_artifacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			number INTEGER NOT NULL,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			detected_on TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			retry_count INTEGER DEFAULT 0,
			event_data JSON,
			UNIQUE (owner, repo, number)
		)`,
	)
	if err != nil {
		return fmt.Errorf("failed to create github_artifacts table: %w", err)
	}
	return nil
}

func createPollStateTable(db *sql.DB) error {
	_, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS poll_state (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			last_polled_at TEXT NOT NULL,
			UNIQUE (owner, repo)
		)`,
	)
	if err != nil {
		return fmt.Errorf("failed to create poll_state table: %w", err)
	}
	return nil
}

func (s *Store) HasEvent(
	owner, repo string,
	number int,
) (bool, error) {
	if owner == "" || repo == "" || number == 0 {
		return false, fmt.Errorf("owner, repo, and number are required")
	}
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM github_artifacts WHERE owner = ? AND repo = ? AND number = ?", owner, repo, number).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if event exists: %w", err)
	}
	return count > 0, nil
}

func (s *Store) IsEventProcessed(
	owner, repo string,
	number int,
) (bool, error) {
	if owner == "" || repo == "" || number == 0 {
		return false, fmt.Errorf("owner, repo, and number are required")
	}
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM github_artifacts WHERE owner = ? AND repo = ? AND number = ? AND status = ?", owner, repo, number, string(StoreStatusProcessed)).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if event exists: %w", err)
	}
	return count > 0, nil
}

func (s *Store) Insert(
	e *event.Event,
	ss StoreStatus,
) error {
	var eventData []byte
	if ss == StoreStatusPendingRetry {
		r, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("failed to marshal event: %w", err)
		}
		eventData = r
	}
	res, err := s.db.Exec("INSERT OR IGNORE INTO github_artifacts (owner, repo, number, type, title, status, detected_on, updated_at, retry_count, event_data) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", e.Owner, e.Repo, e.Number, e.Type, e.Title, string(ss), time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339), 0, eventData)
	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}
	d, err := isDuplicate(res)
	if err != nil {
		return fmt.Errorf("failed to check if duplicate: %w", err)
	}
	if d {
		s.log.Info("No new Row Added", "owner", e.Owner, "repo", e.Repo, "number", e.Number)
		return nil
	}
	return nil
}

func isDuplicate(
	r sql.Result,
) (bool, error) {
	rowsAffected, err := r.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return true, nil
	}
	return false, nil
}

func (s *Store) MarkAs(
	owner, repo string,
	number int,
	ss StoreStatus,
) error {
	_, err := s.db.Exec("UPDATE github_artifacts SET status = ?, updated_at = ? WHERE owner = ? AND repo = ? AND number = ?", string(ss), time.Now().Format(time.RFC3339), owner, repo, number)
	if err != nil {
		return fmt.Errorf("failed to mark as %s: %w", ss, err)
	}
	return nil
}

func (s *Store) IncrementRetryCount(
	owner, repo string,
	number int,
) error {
	_, err := s.db.Exec("UPDATE github_artifacts SET retry_count = retry_count + 1 WHERE owner = ? AND repo = ? AND number = ?", owner, repo, number)
	if err != nil {
		return fmt.Errorf("failed to increment retry count for owner %s, repo %s, number %d: %w", owner, repo, number, err)
	}
	return nil
}

func (s *Store) GetPendingRetryEvents(
	threshold int,
) ([]*event.RetryEvent, error) {
	if threshold <= 0 {
		return nil, fmt.Errorf("threshold must be greater than zero")
	}
	rows, err := s.db.Query("SELECT event_data, retry_count FROM github_artifacts WHERE status = ? AND retry_count < ?", string(StoreStatusPendingRetry), threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending retries: %w", err)
	}
	defer rows.Close()

	events := make([]*event.RetryEvent, 0)
	for rows.Next() {
		var e []byte
		var retryCount int
		err := rows.Scan(&e, &retryCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		var ev event.Event
		err = json.Unmarshal(e, &ev)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal event: %w", err)
		}
		events = append(events, &event.RetryEvent{
			Event:      &ev,
			RetryCount: retryCount,
		})
	}
	return events, nil
}

func (s *Store) GetPollCursor(
	owner, repo string,
) (time.Time, bool, error) {
	var lastPolledAtStr string
	err := s.db.QueryRow("SELECT last_polled_at FROM poll_state WHERE owner = ? AND repo = ?", owner, repo).Scan(&lastPolledAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to query poll cursor: %w", err)
	}
	lastPolledAt, err := time.Parse(time.RFC3339, lastPolledAtStr)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to parse poll cursor time: %w", err)
	}
	return lastPolledAt, true, nil
}

func (s *Store) UpsertPollCursor(
	owner, repo string,
	lastPolledAt time.Time,
) error {
	formattedTime := lastPolledAt.Format(time.RFC3339)
	_, err := s.db.Exec("INSERT INTO poll_state (owner, repo, last_polled_at) VALUES (?, ?, ?) ON CONFLICT (owner, repo) DO UPDATE SET last_polled_at = excluded.last_polled_at", owner, repo, formattedTime)
	if err != nil {
		return fmt.Errorf("failed to upsert poll cursor: %w", err)
	}
	return nil
}
