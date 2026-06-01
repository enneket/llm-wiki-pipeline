package step2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// DedupResult is the result of a dedup check
type DedupResult struct {
	Decision    string // url_duplicate | hash_duplicate | vector_duplicate | kept
	DuplicateOf int64
}

// Dedup checks for duplicate documents
type Dedup struct {
	pool      *pgxpool.Pool
	threshold float64
	enabled   bool
}

// NewDedup creates a new dedup checker
func NewDedup(pool *pgxpool.Pool, threshold float64, enabled bool) *Dedup {
	return &Dedup{pool: pool, threshold: threshold, enabled: enabled}
}

// Check runs all dedup strategies in cascade
func (d *Dedup) Check(ctx context.Context, url, contentHash string, embedding []float32) (*DedupResult, error) {
	// 1. URL exact match
	var existingID int64
	if err := d.pool.QueryRow(ctx, `SELECT id FROM documents WHERE url = $1`, url).Scan(&existingID); err == nil {
		return &DedupResult{Decision: "url_duplicate", DuplicateOf: existingID}, nil
	}

	// 2. Content hash match
	if contentHash != "" {
		var existingID int64
		if err := d.pool.QueryRow(ctx, `SELECT id FROM documents WHERE content_hash = $1`, contentHash).Scan(&existingID); err == nil {
			return &DedupResult{Decision: "hash_duplicate", DuplicateOf: existingID}, nil
		}
	}

	// 3. Vector similarity (if enabled)
	if d.enabled && len(embedding) > 0 {
		sim, dupID, err := d.findSimilar(ctx, embedding)
		if err == nil && sim > float32(d.threshold) {
			return &DedupResult{Decision: "vector_duplicate", DuplicateOf: dupID}, nil
		}
	}

	return &DedupResult{Decision: "kept"}, nil
}

func (d *Dedup) findSimilar(ctx context.Context, embedding []float32) (float32, int64, error) {
	// Find most similar document embedding using cosine distance
	// pgvector cosine distance: 1 - cosine_similarity
	row := d.pool.QueryRow(ctx, `
		SELECT d.id, 1 - (e.embedding <=> $1) AS similarity
		FROM document_embeddings e
		JOIN documents d ON d.id = e.document_id
		WHERE d.source = 'cleaned_raw'
		ORDER BY e.embedding <=> $1
		LIMIT 1
	`, pgvector.NewVector(embedding))

	var id int64
	var similarity float32
	if err := row.Scan(&id, &similarity); err != nil {
		return 0, 0, err
	}
	return similarity, id, nil
}

// ContentHash computes SHA-256 of title+content
func ComputeHash(title, content string) string {
	h := sha256.New()
	h.Write([]byte(title + "\n" + content))
	return hex.EncodeToString(h.Sum(nil))
}

// StoreDedupRecord persists a dedup decision
func StoreDedupRecord(ctx context.Context, pool *pgxpool.Pool, url, contentHash, decision string, duplicateOfID *int64) error {
	var dupID *int64
	if duplicateOfID != nil {
		dupID = duplicateOfID
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO dedup_records (url, content_hash, decision, duplicate_of_id)
		VALUES ($1, $2, $3, $4)
	`, url, contentHash, decision, dupID)
	return err
}
