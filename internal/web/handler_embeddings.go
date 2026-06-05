package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type EmbedState struct {
	Running   bool   `json:"running"`
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
	Current   string `json:"current"`
	cancel    context.CancelFunc `json:"-"`
}

func (s *Server) handleRebuildEmbeddings(w http.ResponseWriter, r *http.Request) {
	if s.processState.Running {
		http.Error(w, `{"error":"LLM processing is running, stop it first"}`, http.StatusConflict)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	s.embedState = EmbedState{Running: true, cancel: cancel}

	go s.rebuildEmbeddings(ctx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *Server) handleEmbedStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.embedState)
}

func (s *Server) handleEmbedStop(w http.ResponseWriter, r *http.Request) {
	if s.embedState.cancel != nil {
		s.embedState.cancel()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopping"})
}

func (s *Server) rebuildEmbeddings(ctx context.Context) {
	pool := s.db.Pool
	llmClient := s.llm

	defer func() {
		s.embedState.Running = false
		s.embedState.Current = ""
	}()

	// Get embedding model
	embedModel := s.cfg.Dedup.Vector.Model
	if embedModel == "" {
		log.Printf("[embed] no embedding model configured")
		return
	}

	// Count wiki pages
	var wikiCount int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM wiki_pages").Scan(&wikiCount)
	if err != nil {
		log.Printf("[embed] failed to count wiki pages: %v", err)
		return
	}

	// Count documents
	var docCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM documents").Scan(&docCount)
	if err != nil {
		log.Printf("[embed] failed to count documents: %v", err)
		return
	}

	s.embedState.Total = wikiCount + docCount
	s.embedState.Completed = 0

	log.Printf("[embed] rebuilding embeddings: %d wiki pages + %d documents", wikiCount, docCount)

	// Process wiki pages
	rows, err := pool.Query(ctx, "SELECT id, title, content FROM wiki_pages ORDER BY id")
	if err != nil {
		log.Printf("[embed] failed to query wiki pages: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		select {
		case <-ctx.Done():
			log.Printf("[embed] stopped by user (%d/%d)", s.embedState.Completed, s.embedState.Total)
			return
		default:
		}

		var id int64
		var title, content string
		if err := rows.Scan(&id, &title, &content); err != nil {
			log.Printf("[embed] failed to scan wiki page: %v", err)
			continue
		}

		s.embedState.Current = title

		// Generate embedding (use title + first 500 chars of content)
		text := title + "\n" + truncateStr(content, 500)
		emb, err := llmClient.EmbedSingle(ctx, text)
		if err != nil {
			log.Printf("[embed] failed to embed wiki page %d (%s): %v", id, title, err)
			s.embedState.Completed++
			continue
		}

		// Store embedding
		_, err = pool.Exec(ctx, `
			INSERT INTO wiki_embeddings (wiki_page_id, embedding, model)
			VALUES ($1, $2, $3)
			ON CONFLICT (wiki_page_id) DO UPDATE SET embedding = EXCLUDED.embedding, model = EXCLUDED.model
		`, id, emb, embedModel)
		if err != nil {
			log.Printf("[embed] failed to store wiki embedding %d: %v", id, err)
		}

		s.embedState.Completed++
		if s.embedState.Completed%10 == 0 {
			log.Printf("[embed] progress: %d/%d", s.embedState.Completed, s.embedState.Total)
		}

		time.Sleep(50 * time.Millisecond) // Rate limit
	}

	// Process documents
	rows2, err := pool.Query(ctx, "SELECT id, title, content FROM documents ORDER BY id")
	if err != nil {
		log.Printf("[embed] failed to query documents: %v", err)
		return
	}
	defer rows2.Close()

	for rows2.Next() {
		select {
		case <-ctx.Done():
			log.Printf("[embed] stopped by user (%d/%d)", s.embedState.Completed, s.embedState.Total)
			return
		default:
		}

		var id int64
		var title, content string
		if err := rows2.Scan(&id, &title, &content); err != nil {
			log.Printf("[embed] failed to scan document: %v", err)
			continue
		}

		s.embedState.Current = title

		// Generate embedding (use title + first 500 chars of content)
		text := title + "\n" + truncateStr(content, 500)
		emb, err := llmClient.EmbedSingle(ctx, text)
		if err != nil {
			log.Printf("[embed] failed to embed document %d (%s): %v", id, title, err)
			s.embedState.Completed++
			continue
		}

		// Store embedding
		_, err = pool.Exec(ctx, `
			INSERT INTO document_embeddings (document_id, embedding, model)
			VALUES ($1, $2, $3)
			ON CONFLICT (document_id) DO UPDATE SET embedding = EXCLUDED.embedding, model = EXCLUDED.model
		`, id, emb, embedModel)
		if err != nil {
			log.Printf("[embed] failed to store doc embedding %d: %v", id, err)
		}

		s.embedState.Completed++
		if s.embedState.Completed%10 == 0 {
			log.Printf("[embed] progress: %d/%d", s.embedState.Completed, s.embedState.Total)
		}

		time.Sleep(50 * time.Millisecond) // Rate limit
	}

	log.Printf("[embed] completed: %d/%d", s.embedState.Completed, s.embedState.Total)
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
