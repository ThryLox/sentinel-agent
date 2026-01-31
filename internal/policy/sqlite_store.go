package policy

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DBStore struct {
	db *sql.DB
}

// NewDBStore opens (or creates) a policies table in the same DB path used by events.
func NewDBStore(dbPath string) (*DBStore, error) {
	// ensure directory exists
	dir := filepath.Dir(dbPath)
	_ = os.MkdirAll(dir, 0o755)
	// _busy_timeout=5000: wait up to 5s for lock
	// _journal_mode=WAL: concurrent readers/writers
	db, err := sql.Open("sqlite", dbPath+"?_foreign_keys=1&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	if err := createPolicyTable(db); err != nil {
		db.Close()
		return nil, err
	}
	return &DBStore{db: db}, nil
}

func createPolicyTable(db *sql.DB) error {
	_, err := db.Exec(`
    PRAGMA journal_mode=WAL;
    CREATE TABLE IF NOT EXISTS policies (
        id TEXT PRIMARY KEY,
        name TEXT,
        raw TEXT NOT NULL,
        updated TEXT NOT NULL
    );`)
	return err
}

// List returns all policies
func (s *DBStore) List() ([]*Policy, error) {
	rows, err := s.db.Query(`SELECT id, name, raw, updated FROM policies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []*Policy
	for rows.Next() {
		var id, name, raw, updated string
		if err := rows.Scan(&id, &name, &raw, &updated); err == nil {
			t, _ := time.Parse(time.RFC3339, updated)
			policies = append(policies, &Policy{ID: id, Name: name, Raw: raw, Updated: t})
		}
	}
	return policies, nil
}

// Get returns the most recently updated policy (or nil)
func (s *DBStore) Get() *Policy {
	row := s.db.QueryRow(`SELECT id, name, raw, updated FROM policies ORDER BY updated DESC LIMIT 1`)
	var id, name, raw, updated string
	if err := row.Scan(&id, &name, &raw, &updated); err != nil {
		return nil
	}
	t, _ := time.Parse(time.RFC3339, updated)
	return &Policy{ID: id, Name: name, Raw: raw, Updated: t}
}

// Set inserts or replaces a policy by id (if id empty, use 'active')
func (s *DBStore) Set(p *Policy) error {
	if p == nil {
		return nil
	}
	id := p.ID
	if id == "" {
		id = "active"
	}
	if p.Updated.IsZero() {
		p.Updated = time.Now().UTC()
	}
	_, err := s.db.Exec(`INSERT INTO policies(id, name, raw, updated) VALUES (?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET name=excluded.name, raw=excluded.raw, updated=excluded.updated`, id, p.Name, p.Raw, p.Updated.Format(time.RFC3339))
	return err
}

func (s *DBStore) Close() error { return s.db.Close() }
