package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"llm-wiki/pkg/llm"
)

type QueryRequest struct {
	Question string `json:"question"`
}

type QueryResponse struct {
	Answer  string         `json:"answer"`
	Sources []llm.SearchResult `json:"sources"`
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

	// Search for relevant wiki pages
	var results []llm.SearchResult
	embedURL := s.llm.EmbedURL()

	if embedURL != "" {
		// Vector search
		embedding, err := s.llm.EmbedSingle(ctx, req.Question)
		if err != nil {
			http.Error(w, fmt.Sprintf("embed error: %v", err), http.StatusInternalServerError)
			return
		}
		results, err = llm.SearchEmbeddings(ctx, s.db.Pool, embedding, 5)
		if err != nil {
			http.Error(w, fmt.Sprintf("search error: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Full-text search fallback
		var err error
		results, err = llm.SearchFullText(ctx, s.db.Pool, req.Question, 5)
		if err != nil {
			http.Error(w, fmt.Sprintf("search error: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Build context from results
	var contextBuilder strings.Builder
	if len(results) == 0 {
		contextBuilder.WriteString("(no related wiki pages found)")
	} else {
		for _, r := range results {
			content := r.Content
			if len(content) > 300 {
				content = content[:300] + "..."
			}
			contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", r.Title, content))
		}
	}

	// Generate answer using LLM
	prompt := fmt.Sprintf(`You are answering a question about a personal wiki.

Question: %s

Related wiki pages:
%s

Based on the related wiki pages above, answer the question. If no relevant information is found, say so.`, req.Question, contextBuilder.String())

	answer, err := s.llm.Complete(ctx, []llm.ChatMessage{
		{Role: "system", Content: "You answer questions based on a personal wiki."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("llm error: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	resp := QueryResponse{
		Answer:  answer,
		Sources: results,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
