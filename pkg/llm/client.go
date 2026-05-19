package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// Client is an OpenAI-compatible LLM client
type Client struct {
	apiKey   string
	baseURL  string
	model    string
	httpClient *http.Client
}

// NewClient creates an LLM client
func NewClient(apiKey, baseURL, model string) *Client {
	return &Client{
		apiKey:   apiKey,
		baseURL:  baseURL,
		model:    model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// CompletionRequest for chat completions
type CompletionRequest struct {
	Model    string          `json:"model"`
	Messages []ChatMessage   `json:"messages"`
	Stream   bool            `json:"stream,omitempty"`
}

// ChatMessage represents a single chat message
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionResponse from OpenAI-compatible API
type CompletionResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Message ChatMessage `json:"message"`
}

type Usage struct {
	InputTokens  int `json:"prompt_tokens"`
	OutputTokens int `json:"completion_tokens"`
}

// Complete sends a chat completion request
func (c *Client) Complete(ctx context.Context, msgs []ChatMessage) (string, error) {
	reqBody := CompletionRequest{
		Model:    c.model,
		Messages: msgs,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return result.Choices[0].Message.Content, nil
}

// EmbedRequest for embeddings
type EmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// EmbedResponse from OpenAI-compatible API
type EmbedResponse struct {
	Data []EmbedData `json:"data"`
}

type EmbedData struct {
	Embedding []float32 `json:"embedding"`
}

// Embed generates embeddings for the given texts
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := EmbedRequest{
		Model: c.model,
		Input: texts,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	embeddings := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}
	return embeddings, nil
}

// EmbedSingle generates embedding for one text
func (c *Client) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	results, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// SearchEmbeddings searches pgvector for top-K similar wiki embeddings
func SearchEmbeddings(ctx context.Context, pool *pgxpool.Pool, queryEmbedding []float32, limit int) ([]SearchResult, error) {
	row := pool.QueryRow(ctx, `
		SELECT w.id, w.title, w.slug, w.content,
		       1 - (e.embedding <=> $1) AS similarity
		FROM wiki_embeddings e
		JOIN wiki_pages w ON w.id = e.wiki_page_id
		WHERE w.page_type IN ('entity', 'concept')
		ORDER BY e.embedding <=> $1
		LIMIT $2
	`, pgvector.NewVector(queryEmbedding), limit)

	var id int64
	var title, slug, content string
	var similarity float32
	if err := row.Scan(&id, &title, &slug, &content, &similarity); err != nil {
		return nil, err
	}
	return []SearchResult{{PageID: id, Title: title, Slug: slug, Content: content, Similarity: similarity}}, nil
}

// SearchResult from vector search
type SearchResult struct {
	PageID     int64
	Title      string
	Slug       string
	Content    string
	Similarity float32
}

// UpsertWikiEmbedding stores a wiki page embedding
func UpsertWikiEmbedding(ctx context.Context, pool *pgxpool.Pool, pageID int64, embedding []float32, model string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO wiki_embeddings (wiki_page_id, embedding, model)
		VALUES ($1, $2, $3)
		ON CONFLICT (wiki_page_id) DO UPDATE SET embedding = EXCLUDED.embedding
	`, pageID, pgvector.NewVector(embedding), model)
	return err
}

// UpsertDocEmbedding stores a document embedding for dedup
func UpsertDocEmbedding(ctx context.Context, pool *pgxpool.Pool, docID int64, embedding []float32, model string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO document_embeddings (document_id, embedding, model)
		VALUES ($1, $2, $3)
		ON CONFLICT (document_id) DO UPDATE SET embedding = EXCLUDED.embedding
	`, docID, pgvector.NewVector(embedding), model)
	return err
}