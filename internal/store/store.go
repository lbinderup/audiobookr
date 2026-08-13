// Package store owns the SQLite database: connection setup, schema
// migrations, and typed queries for settings, jobs and the metadata cache.
package store

import (
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies
// pending migrations. The returned Store is safe for concurrent use; writes
// are serialized by limiting the pool to a single connection, which is plenty
// for a single-user app and sidesteps SQLITE_BUSY entirely.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", url.PathEscape(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }
