package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"audiobookr/internal/metadata"
)

// CacheBook stores the book snapshot for (asin, region), preserving any
// cached chapters.
func (s *Store) CacheBook(asin, region string, book *metadata.Book) error {
	raw, err := json.Marshal(book)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO metadata_cache (asin, region, book_json, fetched_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(asin, region) DO UPDATE SET book_json = excluded.book_json, fetched_at = excluded.fetched_at`,
		asin, region, string(raw), time.Now().UnixMilli())
	return err
}

// CachedBook returns the cached book or nil when absent.
func (s *Store) CachedBook(asin, region string) (*metadata.Book, error) {
	var raw string
	err := s.db.QueryRow(
		"SELECT book_json FROM metadata_cache WHERE asin = ? AND region = ?", asin, region,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && raw == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var b metadata.Book
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return nil, nil // stale/corrupt cache: treat as miss
	}
	return &b, nil
}

// CacheChapters stores chapter data for (asin, region).
func (s *Store) CacheChapters(asin, region string, ch *metadata.ChapterInfo) error {
	raw, err := json.Marshal(ch)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO metadata_cache (asin, region, chapters_json, fetched_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(asin, region) DO UPDATE SET chapters_json = excluded.chapters_json, fetched_at = excluded.fetched_at`,
		asin, region, string(raw), time.Now().UnixMilli())
	return err
}

// CachedChapters returns cached chapters or nil when absent.
func (s *Store) CachedChapters(asin, region string) (*metadata.ChapterInfo, error) {
	var raw string
	err := s.db.QueryRow(
		"SELECT chapters_json FROM metadata_cache WHERE asin = ? AND region = ?", asin, region,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && raw == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ch metadata.ChapterInfo
	if err := json.Unmarshal([]byte(raw), &ch); err != nil {
		return nil, nil
	}
	return &ch, nil
}
