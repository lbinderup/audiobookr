-- The book/chapter cache becomes per-source: the aggregator stores each remote
-- catalog's raw record separately and merges on demand, so per-field user
-- overrides and precedence changes never invalidate anything. Table-rebuild
-- because SQLite cannot add a column to a primary key. Every existing row was
-- fetched from Audnexus (the only source before aggregation existed).
CREATE TABLE metadata_cache_v2 (
    source        TEXT NOT NULL,            -- 'audnexus' | 'audible'
    asin          TEXT NOT NULL,
    region        TEXT NOT NULL,
    book_json     TEXT NOT NULL DEFAULT '',
    chapters_json TEXT NOT NULL DEFAULT '',
    fetched_at    INTEGER NOT NULL,         -- unix ms
    PRIMARY KEY (source, asin, region)
);
INSERT INTO metadata_cache_v2 (source, asin, region, book_json, chapters_json, fetched_at)
    SELECT 'audnexus', asin, region, book_json, chapters_json, fetched_at FROM metadata_cache;
DROP TABLE metadata_cache;
ALTER TABLE metadata_cache_v2 RENAME TO metadata_cache;
