package vector

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"llm-wiki/pkg/llm"
)

// Embedder generates embeddings and searches the wiki
type Embedder struct {
	client *llm.Client
	model  string
	pool   *pgxpool.Pool
}

// NewEmbedder creates a new embedder backed by LLM client and PG pool
func NewEmbedder(client *llm.Client, pool *pgxpool.Pool, model string) *Embedder {
	return &Embedder{client: client, pool: pool, model: model}
}

// Model returns the embedding model name
func (e *Embedder) Model() string {
	return e.model
}

// Embed generates an embedding for the given text
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.client.EmbedSingle(ctx, text)
}

// SearchWiki searches the wiki for relevant pages given a query
func (e *Embedder) SearchWiki(ctx context.Context, query string, topK int) ([]PreSearchResult, error) {
	emb, err := e.client.EmbedSingle(ctx, query)
	if err != nil {
		return nil, err
	}
	results, err := llm.SearchEmbeddings(ctx, e.pool, emb, topK)
	if err != nil {
		return nil, err
	}
	preResults := make([]PreSearchResult, len(results))
	for i, r := range results {
		preResults[i] = PreSearchResult{
			PageID:     r.PageID,
			Title:      r.Title,
			Slug:       r.Slug,
			Similarity: r.Similarity,
			Content:    r.Content,
		}
	}
	return preResults, nil
}

// StoreDocEmbedding stores a document embedding for dedup
func (e *Embedder) StoreDocEmbedding(ctx context.Context, docID int64, text string) error {
	emb, err := e.client.EmbedSingle(ctx, text)
	if err != nil {
		return err
	}
	return llm.UpsertDocEmbedding(ctx, e.pool, docID, emb, e.model)
}

// StoreWikiEmbedding stores a wiki page embedding
func (e *Embedder) StoreWikiEmbedding(ctx context.Context, pageID int64, text string) error {
	emb, err := e.client.EmbedSingle(ctx, text)
	if err != nil {
		return err
	}
	return llm.UpsertWikiEmbedding(ctx, e.pool, pageID, emb, e.model)
}

// PreSearchResult represents a pre-search result from the wiki
type PreSearchResult struct {
	PageID     int64
	Title      string
	Slug       string
	Similarity float32
	Content    string
}
