package events

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
}

func NewSqliteStore(path string) (EventStore, error) {
	db, err := sql.Open("sqlite", path+"?_foreign_keys=1")
	if err != nil {
		return nil, err
	}
	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &sqliteStore{db: db}, nil
}

func createSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS events (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        timestamp TEXT NOT NULL,
        type TEXT NOT NULL,
        payload TEXT NOT NULL
    );`)
	return err
}

func (s *sqliteStore) Save(e Event) error {
	_, err := s.db.Exec(`INSERT INTO events(timestamp, type, payload) VALUES (?, ?, ?)`, e.Timestamp.Format(time.RFC3339), e.Type, e.Payload)
	return err
}

func (s *sqliteStore) List(limit int) ([]Event, error) {
	rows, err := s.db.Query(`SELECT id, timestamp, type, payload FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var id int64
		var ts string
		var typ string
		var payload string
		if err := rows.Scan(&id, &ts, &typ, &payload); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, ts)
		out = append(out, Event{ID: id, Timestamp: t, Type: typ, Payload: payload})
	}
	return out, nil
}

func (s *sqliteStore) Close() error { return s.db.Close() }
