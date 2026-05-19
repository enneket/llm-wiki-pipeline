-- llm-wiki pipeline schema migration
-- Run: psql -d $DATABASE_URL -f migrations/001_init.sql

-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Feeds table: RSS/Atom subscription sources
CREATE TABLE IF NOT EXISTS feeds (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    url         TEXT NOT NULL,
    tags        TEXT[] NOT NULL DEFAULT '{}',
    interval    TEXT NOT NULL,
    last_fetch  TIMESTAMPTZ,
    last_build  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Documents table: raw + cleaned documents
CREATE TABLE IF NOT EXISTS documents (
    id              BIGSERIAL PRIMARY KEY,
    feed_id         BIGINT REFERENCES feeds(id) ON DELETE SET NULL,
    url             TEXT NOT NULL,
    title           TEXT NOT NULL,
    content_hash    TEXT NOT NULL,
    content         TEXT NOT NULL,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    source          TEXT NOT NULL,           -- 'raw' | 'cleaned_raw' | 'reject'
    file_path       TEXT,                    -- relative path under data/
    confidence      REAL,                    -- filter confidence score
    judged_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- URL exact dedup index
CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_url ON documents(url);

-- Content hash dedup index (partial for speed)
CREATE INDEX IF NOT EXISTS idx_documents_content_hash ON documents(content_hash);

-- Vector dedup: documents with embeddings in pgvector
CREATE TABLE IF NOT EXISTS document_embeddings (
    id              BIGSERIAL PRIMARY KEY,
    document_id     BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    embedding       VECTOR(1536),            -- OpenAI text-embedding-3-small dim
    model           TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_document_embeddings_vector
    ON document_embeddings USING ivfflat (embedding vector_cosine_ops);

-- Wiki pages table
CREATE TABLE IF NOT EXISTS wiki_pages (
    id              BIGSERIAL PRIMARY KEY,
    title           TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    page_type       TEXT NOT NULL,           -- 'entity' | 'concept' | 'source'
    tags            TEXT[] NOT NULL DEFAULT '{}',
    content         TEXT NOT NULL,
    sources         TEXT[] NOT NULL DEFAULT '{}',  -- cleaned_raw file paths
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_modified   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_slug ON wiki_pages(slug);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_tags ON wiki_pages USING gin(tags);

-- Wiki page embeddings for wikilink pre-search
CREATE TABLE IF NOT EXISTS wiki_embeddings (
    id              BIGSERIAL PRIMARY KEY,
    wiki_page_id    BIGINT NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    embedding       VECTOR(1536),
    model           TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wiki_embeddings_vector
    ON wiki_embeddings USING ivfflat (embedding vector_cosine_ops);

-- Dedup records: track dedup decisions
CREATE TABLE IF NOT EXISTS dedup_records (
    id              BIGSERIAL PRIMARY KEY,
    url             TEXT,
    content_hash    TEXT,
    decision        TEXT NOT NULL,           -- 'url_duplicate' | 'hash_duplicate' | 'vector_duplicate' | 'kept'
    duplicate_of_id BIGINT REFERENCES documents(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Ingest queue: async ingest job queue
CREATE TABLE IF NOT EXISTS ingest_queue (
    id              BIGSERIAL PRIMARY KEY,
    document_id     BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending',   -- 'pending' | 'processing' | 'done' | 'failed'
    attempts        INT NOT NULL DEFAULT 0,
    error           TEXT,
    queued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ingest_queue_status ON ingest_queue(status);

-- Config version tracker (for hot reload)
CREATE TABLE IF NOT EXISTS config_version (
    key             TEXT PRIMARY KEY,
    version         INT NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);