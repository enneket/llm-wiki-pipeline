package web

import (
	"encoding/json"
	"net/http"
)

type StatusResponse struct {
	Feeds      int `json:"feeds"`
	Documents  int `json:"documents"`
	WikiPages  int `json:"wiki_pages"`
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Failed     int `json:"failed"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var status StatusResponse

	// Count feeds
	err := s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM feeds").Scan(&status.Feeds)
	if err != nil {
		http.Error(w, "failed to count feeds", http.StatusInternalServerError)
		return
	}

	// Count documents
	err = s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM documents").Scan(&status.Documents)
	if err != nil {
		http.Error(w, "failed to count documents", http.StatusInternalServerError)
		return
	}

	// Count wiki pages
	err = s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM wiki_pages").Scan(&status.WikiPages)
	if err != nil {
		http.Error(w, "failed to count wiki pages", http.StatusInternalServerError)
		return
	}

	// Count queue status
	err = s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM ingest_queue WHERE status = 'pending'").Scan(&status.Pending)
	if err != nil {
		http.Error(w, "failed to count pending", http.StatusInternalServerError)
		return
	}

	err = s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM ingest_queue WHERE status = 'processing'").Scan(&status.Processing)
	if err != nil {
		http.Error(w, "failed to count processing", http.StatusInternalServerError)
		return
	}

	err = s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM ingest_queue WHERE status = 'failed'").Scan(&status.Failed)
	if err != nil {
		http.Error(w, "failed to count failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
