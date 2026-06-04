package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// Client is an OpenAI-compatible LLM client
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	provider   string // "openai" or "volcengine"
	embedURL   string
	embedKey   string
	httpClient *http.Client
}

// NewClient creates an LLM client, auto-detecting provider from baseURL
func NewClient(apiKey, baseURL, model string) *Client {
	provider := detectProvider(baseURL)
	// Strip the API path from baseURL so we can append the correct path per provider
	cleanBase := cleanBaseURL(baseURL, provider)
	return &Client{
		apiKey:     apiKey,
		baseURL:    cleanBase,
		model:      model,
		provider:   provider,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// EmbedURL returns the configured embedding endpoint URL
func (c *Client) EmbedURL() string {
	return c.embedURL
}

// NewClientWithEmbed creates an LLM client with separate embedding endpoint
func NewClientWithEmbed(apiKey, baseURL, model, embedURL, embedKey string) *Client {
	provider := detectProvider(baseURL)
	cleanBase := cleanBaseURL(baseURL, provider)
	return &Client{
		apiKey:     apiKey,
		baseURL:    cleanBase,
		model:      model,
		provider:   provider,
		embedURL:   cleanBaseURL(embedURL, provider),
		embedKey:   embedKey,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// detectProvider determines the API provider from baseURL
func detectProvider(baseURL string) string {
	lower := strings.ToLower(baseURL)
	if strings.Contains(lower, "volc") {
		return "volcengine"
	}
	if strings.Contains(lower, "anthropic") {
		return "anthropic"
	}
	return "openai"
}

// cleanBaseURL strips the API path suffix from baseURL so paths can be appended cleanly
func cleanBaseURL(baseURL, provider string) string {
	baseURL = strings.TrimSuffix(baseURL, "/")
	switch provider {
	case "volcengine":
		return strings.TrimSuffix(baseURL, "/api/v3/responses")
	case "anthropic":
		return strings.TrimSuffix(baseURL, "/v1/messages")
	default:
		return strings.TrimSuffix(baseURL, "/v1")
	}
}

// --- OpenAI request/response types ---

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	ID      string   `json:"id"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Message chatMessage `json:"message"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// --- Volcengine request/response types ---

type volcInputItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type volcRequest struct {
	Model string          `json:"model"`
	Input []volcInputItem `json:"input"`
}

type volcContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type volcMessage struct {
	Role    string             `json:"role"`
	Content []volcContentBlock `json:"content"`
}

type volcOutput struct {
	Type    string             `json:"type"`
	Role    string             `json:"role,omitempty"`
	Content []volcContentBlock `json:"content,omitempty"`
	Message volcMessage        `json:"message,omitempty"`
}

type volcResponse struct {
	Output []volcOutput `json:"output"`
}

// Complete sends a chat completion request
func (c *Client) Complete(ctx context.Context, msgs []ChatMessage) (string, error) {
	if c.provider == "volcengine" {
		return c.completeVolcengine(ctx, msgs)
	}
	if c.provider == "anthropic" {
		return c.completeAnthropic(ctx, msgs)
	}
	return c.completeOpenAI(ctx, msgs)
}

func (c *Client) completeOpenAI(ctx context.Context, msgs []ChatMessage) (string, error) {
	reqBody := chatCompletionRequest{
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

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return result.Choices[0].Message.Content, nil
}

func (c *Client) completeVolcengine(ctx context.Context, msgs []ChatMessage) (string, error) {
	// Convert messages to Volcengine input format
	input := make([]volcInputItem, len(msgs))
	for i, m := range msgs {
		input[i] = volcInputItem{Role: m.Role, Content: m.Content}
	}
	reqBody := volcRequest{Model: c.model, Input: input}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v3/responses", bytes.NewReader(data))
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

	var result volcResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	for _, out := range result.Output {
		if out.Type == "message" {
			// Try direct content first, then nested message.content
			blocks := out.Content
			if len(blocks) == 0 {
				blocks = out.Message.Content
			}
			for _, block := range blocks {
				if block.Type == "output_text" {
					return block.Text, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no text in volcengine response")
}

// --- Anthropic request/response types ---

type anthropicRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func (c *Client) completeAnthropic(ctx context.Context, msgs []ChatMessage) (string, error) {
	// Convert messages to Anthropic format
	messages := make([]anthropicMessage, len(msgs))
	for i, m := range msgs {
		messages[i] = anthropicMessage{Role: m.Role, Content: m.Content}
	}
	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		Messages:  messages,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
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

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("no text in anthropic response")
}

// ChatMessage represents a single chat message (exported for use in completeOpenAI)
type ChatMessage = chatMessage

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
	if c.embedURL == "" {
		return nil, fmt.Errorf("embedding not configured: EMBEDDING_BASE_URL is not set")
	}

	reqBody := EmbedRequest{
		Model: c.model,
		Input: texts,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	embedURL := c.embedURL
	embedKey := c.embedKey
	if embedURL == "" {
		return nil, fmt.Errorf("embedding endpoint not configured: EMBEDDING_BASE_URL is empty")
	}
	if embedKey == "" {
		embedKey = c.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embedURL+"/v1/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+embedKey)
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
	rows, err := pool.Query(ctx, `
		SELECT w.id, w.title, w.slug, w.content,
		       1 - (e.embedding <=> $1) AS similarity
		FROM wiki_embeddings e
		JOIN wiki_pages w ON w.id = e.wiki_page_id
		WHERE w.page_type IN ('entity', 'concept')
		ORDER BY e.embedding <=> $1
		LIMIT $2
	`, pgvector.NewVector(queryEmbedding), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.PageID, &r.Title, &r.Slug, &r.Content, &r.Similarity); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// SearchResult from vector search
type SearchResult struct {
	PageID     int64
	Title      string
	Slug       string
	Content    string
	Similarity float32
}

// SearchFullText searches wiki pages by text match (fallback when embedding not configured)
func SearchFullText(ctx context.Context, pool *pgxpool.Pool, query string, limit int) ([]SearchResult, error) {
	rows, err := pool.Query(ctx, `
		SELECT w.id, w.title, w.slug, w.content
		FROM wiki_pages w
		WHERE w.page_type IN ('entity', 'concept')
		  AND (w.title ILIKE $1 OR w.content ILIKE $1)
		LIMIT $2
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.PageID, &r.Title, &r.Slug, &r.Content); err != nil {
			return nil, err
		}
		r.Similarity = 0.5 // neutral score for text match
		results = append(results, r)
	}
	return results, nil
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
