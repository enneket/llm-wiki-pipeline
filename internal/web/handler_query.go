package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"llm-wiki/internal/search"
	"llm-wiki/pkg/llm"
)

type QueryRequest struct {
	Question       string `json:"question"`
	ConversationID int64  `json:"conversation_id,omitempty"`
}

type QueryResponse struct {
	Answer         string             `json:"answer"`
	Sources        []llm.SearchResult `json:"sources"`
	Mode           string             `json:"mode"`
	ConversationID int64              `json:"conversation_id"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Question == "" {
		http.Error(w, "question is required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Determine if vector search is available
	embedURL := s.llm.EmbedURL()
	vectorEnabled := embedURL != ""

	// Get query embedding if vector search is enabled
	var queryEmbedding []float32
	if vectorEnabled {
		emb, err := s.llm.EmbedSingle(ctx, req.Question)
		if err != nil {
			// Non-fatal: fall back to keyword search
			vectorEnabled = false
		} else {
			queryEmbedding = emb
		}
	}

	// Perform hybrid search
	hybridResult, err := search.HybridSearch(ctx, s.db.Pool, req.Question, queryEmbedding, search.VectorSearchConfig{
		Enabled: vectorEnabled,
		TopK:    20,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("search error: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to legacy format for response
	var results []llm.SearchResult
	for _, r := range hybridResult.Results {
		results = append(results, llm.SearchResult{
			PageID:     r.ID,
			Title:      r.Title,
			Slug:       r.Slug,
			Content:    r.Content,
			Similarity: r.Score,
		})
	}

	// Build context from results - use matched chunks if available
	var contextBuilder strings.Builder
	if len(hybridResult.Results) == 0 {
		contextBuilder.WriteString("(no related wiki pages found)")
	} else {
		for _, r := range hybridResult.Results {
			if len(r.MatchedChunks) > 0 {
				// Use matched chunks for more precise context
				contextBuilder.WriteString(fmt.Sprintf("- %s:\n", r.Title))
				for _, chunk := range r.MatchedChunks {
					text := chunk.Text
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					if chunk.HeadingPath != "" {
						contextBuilder.WriteString(fmt.Sprintf("  [%s] %s\n", chunk.HeadingPath, text))
					} else {
						contextBuilder.WriteString(fmt.Sprintf("  %s\n", text))
					}
				}
			} else {
				content := r.Content
				if len(content) > 300 {
					content = content[:300] + "..."
				}
				contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", r.Title, content))
			}
		}
	}

	// Generate answer using LLM
	// Build conversation history if available
	var messages []llm.ChatMessage
	messages = append(messages, llm.ChatMessage{
		Role:    "system",
		Content: "You answer questions based on a personal wiki. Be helpful and provide detailed answers based on the wiki content.",
	})

	// Load conversation history if conversation_id is provided
	if req.ConversationID > 0 {
		rows, err := s.db.Pool.Query(ctx, `
			SELECT question, answer FROM query_history
			WHERE conversation_id = $1
			ORDER BY created_at ASC
			LIMIT 10
		`, req.ConversationID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var q, a string
				if err := rows.Scan(&q, &a); err == nil {
					messages = append(messages, llm.ChatMessage{Role: "user", Content: q})
					messages = append(messages, llm.ChatMessage{Role: "assistant", Content: a})
				}
			}
		}
	}

	// Add current question with wiki context
	prompt := fmt.Sprintf(`Based on the following wiki pages, answer the question.

Related wiki pages:
%s

Question: %s`, contextBuilder.String(), req.Question)

	messages = append(messages, llm.ChatMessage{Role: "user", Content: prompt})

	answer, err := s.llm.Complete(ctx, messages)
	if err != nil {
		http.Error(w, fmt.Sprintf("llm error: %v", err), http.StatusInternalServerError)
		return
	}

	// Create or use existing conversation
	conversationID := req.ConversationID
	if conversationID == 0 {
		// Create new conversation with first question as title
		title := req.Question
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		err = s.db.Pool.QueryRow(ctx, `
			INSERT INTO conversations (title) VALUES ($1) RETURNING id
		`, title).Scan(&conversationID)
		if err != nil {
			// Non-fatal
			conversationID = 0
		}
	} else {
		// Update conversation updated_at
		s.db.Pool.Exec(ctx, `UPDATE conversations SET updated_at = NOW() WHERE id = $1`, conversationID)
	}

	// Save to history
	sourcesJSON, _ := json.Marshal(results)
	_, err = s.db.Pool.Exec(ctx, `
		INSERT INTO query_history (question, answer, sources, conversation_id)
		VALUES ($1, $2, $3, $4)
	`, req.Question, answer, sourcesJSON, conversationID)
	if err != nil {
		// Log but don't fail the request
		_ = err
	}

	// Return response
	resp := QueryResponse{
		Answer:         answer,
		Sources:        results,
		Mode:           hybridResult.Mode,
		ConversationID: conversationID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleQueryHistory returns recent query history
func (s *Server) handleQueryHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, question, LEFT(answer, 200), sources, created_at::text
		FROM query_history
		ORDER BY created_at DESC
		LIMIT 50
	`)
	if err != nil {
		http.Error(w, "failed to query history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type HistoryItem struct {
		ID        int64           `json:"id"`
		Question  string          `json:"question"`
		Answer    string          `json:"answer"`
		Sources   json.RawMessage `json:"sources"`
		CreatedAt string          `json:"created_at"`
	}

	var items []HistoryItem
	for rows.Next() {
		var item HistoryItem
		if err := rows.Scan(&item.ID, &item.Question, &item.Answer, &item.Sources, &item.CreatedAt); err != nil {
			continue
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// handleQueryHistoryDetail returns a single query history item
func (s *Server) handleQueryHistoryDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	type HistoryDetail struct {
		ID        int64           `json:"id"`
		Question  string          `json:"question"`
		Answer    string          `json:"answer"`
		Sources   json.RawMessage `json:"sources"`
		CreatedAt string          `json:"created_at"`
	}

	var item HistoryDetail
	err = s.db.Pool.QueryRow(ctx, `
		SELECT id, question, answer, sources, created_at::text
		FROM query_history
		WHERE id = $1
	`, id).Scan(&item.ID, &item.Question, &item.Answer, &item.Sources, &item.CreatedAt)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}
