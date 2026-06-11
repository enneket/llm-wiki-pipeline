package web

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type Conversation struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	MessageCount int `json:"message_count"`
}

type ConversationMessage struct {
	ID        int64           `json:"id"`
	Role      string          `json:"role"` // "user" | "assistant"
	Content   string          `json:"content"`
	Sources   json.RawMessage `json:"sources,omitempty"`
	CreatedAt string          `json:"created_at"`
}

// handleCreateConversation creates a new conversation
func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Title = "新对话"
	}
	if req.Title == "" {
		req.Title = "新对话"
	}

	var id int64
	err := s.db.Pool.QueryRow(ctx, `
		INSERT INTO conversations (title) VALUES ($1) RETURNING id
	`, req.Title).Scan(&id)
	if err != nil {
		http.Error(w, "failed to create conversation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "title": req.Title})
}

// handleListConversations returns all conversations
func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool.Query(ctx, `
		SELECT c.id, c.title, c.created_at::text, c.updated_at::text,
			   (SELECT COUNT(*) FROM query_history q WHERE q.conversation_id = c.id) as message_count
		FROM conversations c
		ORDER BY c.updated_at DESC
		LIMIT 100
	`)
	if err != nil {
		http.Error(w, "failed to query conversations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var conversations []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt, &c.MessageCount); err != nil {
			continue
		}
		conversations = append(conversations, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conversations)
}

// handleGetConversation returns a conversation with all messages
func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid conversation id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get conversation info
	var conv Conversation
	err = s.db.Pool.QueryRow(ctx, `
		SELECT id, title, created_at::text, updated_at::text
		FROM conversations WHERE id = $1
	`, id).Scan(&conv.ID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		http.Error(w, "conversation not found", http.StatusNotFound)
		return
	}

	// Get messages
	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, question, answer, sources, created_at::text
		FROM query_history
		WHERE conversation_id = $1
		ORDER BY created_at ASC
	`, id)
	if err != nil {
		http.Error(w, "failed to query messages", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var messages []ConversationMessage
	for rows.Next() {
		var qID int64
		var question, answer, createdAt string
		var sources json.RawMessage
		if err := rows.Scan(&qID, &question, &answer, &sources, &createdAt); err != nil {
			continue
		}
		// Add user message
		messages = append(messages, ConversationMessage{
			ID:        qID,
			Role:      "user",
			Content:   question,
			CreatedAt: createdAt,
		})
		// Add assistant message
		messages = append(messages, ConversationMessage{
			ID:        qID,
			Role:      "assistant",
			Content:   answer,
			Sources:   sources,
			CreatedAt: createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"conversation": conv,
		"messages":     messages,
	})
}

// handleDeleteConversation deletes a conversation
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid conversation id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	_, err = s.db.Pool.Exec(ctx, "DELETE FROM conversations WHERE id = $1", id)
	if err != nil {
		http.Error(w, "failed to delete conversation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}
