package search

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// ChunkSearchResult represents a single chunk search result
type ChunkSearchResult struct {
	ChunkID     int64   `json:"chunk_id"`
	PageID      int64   `json:"page_id"`
	ChunkIndex  int     `json:"chunk_index"`
	ChunkText   string  `json:"chunk_text"`
	HeadingPath string  `json:"heading_path"`
	Score       float32 `json:"score"`
}

// PageSearchResult represents a page-level search result
type PageSearchResult struct {
	ID             int64              `json:"id"`
	Title          string             `json:"title"`
	Slug           string             `json:"slug"`
	Content        string             `json:"content"`
	Score          float32            `json:"score"`
	MatchedChunks  []ChunkMatch       `json:"matched_chunks,omitempty"`
}

// ChunkMatch represents a matched chunk within a page
type ChunkMatch struct {
	Text        string  `json:"text"`
	HeadingPath string  `json:"heading_path"`
	Score       float32 `json:"score"`
}

// HybridSearchResult represents the final hybrid search result
type HybridSearchResult struct {
	Mode       string             `json:"mode"` // "keyword" | "vector" | "hybrid"
	Results    []PageSearchResult `json:"results"`
	TokenHits  int                `json:"token_hits"`
	VectorHits int                `json:"vector_hits"`
}

// VectorSearchConfig holds vector search configuration
type VectorSearchConfig struct {
	Enabled bool
	TopK    int
}

// SearchWithChunks performs vector search over chunks and aggregates by page
func SearchWithChunks(ctx context.Context, pool *pgxpool.Pool, queryEmbedding []float32, topK int) ([]PageSearchResult, error) {
	// Over-fetch chunks to get better page-level results
	chunkTopK := topK * 3
	if chunkTopK < 30 {
		chunkTopK = 30
	}

	rows, err := pool.Query(ctx, `
		SELECT 
			c.id, c.wiki_page_id, c.chunk_index, c.chunk_text, c.heading_path,
			1 - (c.embedding <=> $1) AS score
		FROM wiki_chunks c
		ORDER BY c.embedding <=> $1
		LIMIT $2
	`, pgvector.NewVector(queryEmbedding), chunkTopK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []ChunkSearchResult
	for rows.Next() {
		var c ChunkSearchResult
		if err := rows.Scan(&c.ChunkID, &c.PageID, &c.ChunkIndex, &c.ChunkText, &c.HeadingPath, &c.Score); err != nil {
			continue
		}
		chunks = append(chunks, c)
	}

	if len(chunks) == 0 {
		return nil, nil
	}

	// Group chunks by page
	pageChunks := make(map[int64][]ChunkSearchResult)
	for _, c := range chunks {
		pageChunks[c.PageID] = append(pageChunks[c.PageID], c)
	}

	// Calculate page-level scores using max-pool + weighted tail
	var results []PageSearchResult
	for pageID, chunks := range pageChunks {
		// Sort chunks by score descending
		sortChunksByScore(chunks)

		// Get page info
		var title, slug, content string
		err := pool.QueryRow(ctx, `
			SELECT title, slug, LEFT(content, 500) FROM wiki_pages WHERE id = $1
		`, pageID).Scan(&title, &slug, &content)
		if err != nil {
			continue
		}

		// Calculate blended score: max + 0.3 * sum(others), capped at 1.0
		topScore := chunks[0].Score
		tailSum := float32(0)
		for _, c := range chunks[1:] {
			tailSum += c.Score
		}
		blended := topScore + min32(tailSum*0.3, max32(0, 1-topScore))

		// Build matched chunks (top 3)
		matchedChunks := make([]ChunkMatch, 0, min(len(chunks), 3))
		for i := 0; i < min(len(chunks), 3); i++ {
			matchedChunks = append(matchedChunks, ChunkMatch{
				Text:        chunks[i].ChunkText,
				HeadingPath: chunks[i].HeadingPath,
				Score:       chunks[i].Score,
			})
		}

		results = append(results, PageSearchResult{
			ID:            pageID,
			Title:         title,
			Slug:          slug,
			Content:       content,
			Score:         blended,
			MatchedChunks: matchedChunks,
		})
	}

	// Sort by score descending
	sortPagesByScore(results)

	// Return top K
	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// KeywordSearch performs full-text search on wiki pages
func KeywordSearch(ctx context.Context, pool *pgxpool.Pool, query string, topK int) ([]PageSearchResult, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, title, slug, LEFT(content, 500), 
			   ts_rank_cd(to_tsvector('simple', title || ' ' || content), plainto_tsquery('simple', $1)) AS score
		FROM wiki_pages
		WHERE to_tsvector('simple', title || ' ' || content) @@ plainto_tsquery('simple', $1)
		OR title ILIKE '%' || $1 || '%'
		ORDER BY score DESC
		LIMIT $2
	`, query, topK)
	if err != nil {
		// Fallback to ILIKE if full-text search fails
		rows, err = pool.Query(ctx, `
			SELECT id, title, slug, LEFT(content, 500), 0.5 AS score
			FROM wiki_pages
			WHERE title ILIKE '%' || $1 || '%'
			OR content ILIKE '%' || $1 || '%'
			LIMIT $2
		`, query, topK)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var results []PageSearchResult
	for rows.Next() {
		var p PageSearchResult
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Content, &p.Score); err != nil {
			continue
		}
		results = append(results, p)
	}

	return results, nil
}

// HybridSearch combines keyword and vector search using RRF
func HybridSearch(ctx context.Context, pool *pgxpool.Pool, query string, queryEmbedding []float32, cfg VectorSearchConfig) (*HybridSearchResult, error) {
	const topK = 20
	const rrfK = 60 // RRF constant

	result := &HybridSearchResult{}

	// Always do keyword search
	keywordResults, err := KeywordSearch(ctx, pool, query, topK*2)
	if err != nil {
		return nil, err
	}
	result.TokenHits = len(keywordResults)

	// If vector search is enabled and we have an embedding
	if cfg.Enabled && len(queryEmbedding) > 0 {
		vectorResults, err := SearchWithChunks(ctx, pool, queryEmbedding, topK*2)
		if err == nil {
			result.VectorHits = len(vectorResults)
			result.Mode = "hybrid"

			// RRF fusion
			rrfScores := make(map[int64]float64)
			pageData := make(map[int64]*PageSearchResult)

			// Add keyword results
			for rank, p := range keywordResults {
				rrfScores[p.ID] += 1.0 / (float64(rrfK) + float64(rank))
				if _, exists := pageData[p.ID]; !exists {
					pageCopy := p
					pageData[p.ID] = &pageCopy
				}
			}

			// Add vector results
			for rank, p := range vectorResults {
				rrfScores[p.ID] += 1.0 / (float64(rrfK) + float64(rank))
				if existing, exists := pageData[p.ID]; exists {
					// Merge matched chunks
					existing.MatchedChunks = append(existing.MatchedChunks, p.MatchedChunks...)
				} else {
					pageCopy := p
					pageData[p.ID] = &pageCopy
				}
			}

			// Build final results
			for id, score := range rrfScores {
				if page, exists := pageData[id]; exists {
					page.Score = float32(score)
					result.Results = append(result.Results, *page)
				}
			}

			// Sort by RRF score
			sortPagesByScore(result.Results)

			// Return top K
			if len(result.Results) > topK {
				result.Results = result.Results[:topK]
			}

			return result, nil
		}
	}

	// Fallback to keyword-only
	result.Mode = "keyword"
	result.Results = keywordResults
	if len(result.Results) > topK {
		result.Results = result.Results[:topK]
	}

	return result, nil
}

func sortChunksByScore(chunks []ChunkSearchResult) {
	for i := 1; i < len(chunks); i++ {
		for j := i; j > 0 && chunks[j].Score > chunks[j-1].Score; j-- {
			chunks[j], chunks[j-1] = chunks[j-1], chunks[j]
		}
	}
}

func sortPagesByScore(pages []PageSearchResult) {
	for i := 1; i < len(pages); i++ {
		for j := i; j > 0 && pages[j].Score > pages[j-1].Score; j-- {
			pages[j], pages[j-1] = pages[j-1], pages[j]
		}
	}
}

func min32(a, b float32) float32 {
	return float32(math.Min(float64(a), float64(b)))
}

func max32(a, b float32) float32 {
	return float32(math.Max(float64(a), float64(b)))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
