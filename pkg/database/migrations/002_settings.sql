-- Settings table for DB-backed configuration
CREATE TABLE IF NOT EXISTS settings (
    category   TEXT PRIMARY KEY,
    data       JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
