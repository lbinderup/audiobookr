CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE jobs (
    id            TEXT PRIMARY KEY,
    status        TEXT NOT NULL DEFAULT 'pending', -- pending|running|done|error|cancelled
    stage         TEXT NOT NULL DEFAULT '',        -- probe|plan|merge|chapters|tag|move|verify|cleanup
    progress      REAL NOT NULL DEFAULT 0,         -- 0..1
    input_path    TEXT NOT NULL,                   -- absolute source path
    source_files  TEXT NOT NULL DEFAULT '[]',      -- JSON array: resolved merge order
    asin          TEXT NOT NULL,
    region        TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',      -- snapshot of resolved book metadata
    options_json  TEXT NOT NULL DEFAULT '{}',      -- snapshot of conversion options
    chapters_json TEXT NOT NULL DEFAULT '',        -- chapters actually embedded + their source
    output_path   TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    warnings      TEXT NOT NULL DEFAULT '[]',      -- JSON array of user-facing warnings
    retried_from  TEXT NOT NULL DEFAULT '',
    log_path      TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,                -- unix ms
    started_at    INTEGER,
    finished_at   INTEGER
);
CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_created ON jobs(created_at DESC);

CREATE TABLE metadata_cache (
    asin          TEXT NOT NULL,
    region        TEXT NOT NULL,
    book_json     TEXT NOT NULL DEFAULT '',
    chapters_json TEXT NOT NULL DEFAULT '',
    fetched_at    INTEGER NOT NULL,
    PRIMARY KEY (asin, region)
);
