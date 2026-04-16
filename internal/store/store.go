package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

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
	var path string

	if os.Getenv("AMGI_DB_PATH") == "" {
		path = "/etc/amgi/amgi.db"
	} else {
		path = os.Getenv("AMGI_DB_PATH")
	}
	db, err := sql.Open("sqlite", path)
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
