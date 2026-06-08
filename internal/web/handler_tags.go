package web

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type SuggestedTag struct {
	ID          int64  `json:"id"`
	Tag         string `json:"tag"`
	SourceCount int    `json:"source_count"`
	FirstSeen   string `json:"first_seen"`
	LastSeen    string `json:"last_seen"`
	Status      string `json:"status"`
}

// handleListSuggestedTags returns all pending suggested tags
func (s *Server) handleListSuggestedTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, tag, source_count, first_seen::text, last_seen::text, status
		FROM suggested_tags
		WHERE status = 'pending'
		ORDER BY source_count DESC, last_seen DESC
		LIMIT 100
	`)
	if err != nil {
		http.Error(w, "failed to query suggested tags", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tags []SuggestedTag
	for rows.Next() {
		var t SuggestedTag
		if err := rows.Scan(&t.ID, &t.Tag, &t.SourceCount, &t.FirstSeen, &t.LastSeen, &t.Status); err != nil {
			continue
		}
		tags = append(tags, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

// handleAcceptSuggestedTag accepts a suggested tag and adds it to filter tags
func (s *Server) handleAcceptSuggestedTag(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid tag id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get the tag
	var tag string
	err = s.db.Pool.QueryRow(ctx, "SELECT tag FROM suggested_tags WHERE id = $1", id).Scan(&tag)
	if err != nil {
		http.Error(w, "tag not found", http.StatusNotFound)
		return
	}

	// Mark as accepted
	_, err = s.db.Pool.Exec(ctx, "UPDATE suggested_tags SET status = 'accepted' WHERE id = $1", id)
	if err != nil {
		http.Error(w, "failed to accept tag", http.StatusInternalServerError)
		return
	}

	// Add to filter tags in config
	cfg := s.cfg
	if cfg != nil {
		// Check if tag already exists
		exists := false
		for _, t := range cfg.Filter.Keyword.Tags {
			if t == tag {
				exists = true
				break
			}
		}
		if !exists {
			cfg.Filter.Keyword.Tags = append(cfg.Filter.Keyword.Tags, tag)
			// Save to database
			if err := s.reloadConfig(ctx); err != nil {
				// Log but don't fail
				_ = err
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "tag": tag})
}

// handleRejectSuggestedTag rejects a suggested tag
func (s *Server) handleRejectSuggestedTag(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid tag id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	_, err = s.db.Pool.Exec(ctx, "UPDATE suggested_tags SET status = 'rejected' WHERE id = $1", id)
	if err != nil {
		http.Error(w, "failed to reject tag", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
}

// handleSuggestedTagsStats returns stats about suggested tags
func (s *Server) handleSuggestedTagsStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var pending, accepted, rejected int
	err := s.db.Pool.QueryRow(ctx, `
		SELECT 
			COUNT(*) FILTER (WHERE status = 'pending'),
			COUNT(*) FILTER (WHERE status = 'accepted'),
			COUNT(*) FILTER (WHERE status = 'rejected')
		FROM suggested_tags
	`).Scan(&pending, &accepted, &rejected)
	if err != nil {
		http.Error(w, "failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"pending":  pending,
		"accepted": accepted,
		"rejected": rejected,
	})
}
