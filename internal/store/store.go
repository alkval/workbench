package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Event struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	ServiceID string    `json:"service_id,omitempty"`
	Action    string    `json:"action"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY,
			timestamp TEXT NOT NULL,
			service_id TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_service_timestamp ON events(service_id, timestamp DESC)`,
		`PRAGMA optimize`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize sqlite: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Add(ctx context.Context, serviceID, action, level, message string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events(timestamp, service_id, action, level, message) VALUES (?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339Nano), serviceID, action, level, message,
	)
	return err
}

func (s *Store) Recent(ctx context.Context, limit int) ([]Event, error) {
	if limit < 1 || limit > 200 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, timestamp, service_id, action, level, message FROM events ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0, limit)
	for rows.Next() {
		var event Event
		var timestamp string
		if err := rows.Scan(&event.ID, &timestamp, &event.ServiceID, &event.Action, &event.Level, &event.Message); err != nil {
			return nil, err
		}
		event.Timestamp, _ = time.Parse(time.RFC3339Nano, timestamp)
		events = append(events, event)
	}
	return events, rows.Err()
}
