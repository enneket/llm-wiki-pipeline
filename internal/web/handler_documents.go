package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type DocumentItem struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	FeedName  string   `json:"feed_name"`
	Tags      []string `json:"tags"`
	Source    string   `json:"source"`
	Status    string   `json:"status"`
	Summary   string   `json:"summary"`
	Published *string  `json:"published,omitempty"`
	CreatedAt string   `json:"created_at"`
}

type DocumentDetail struct {
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	FeedName   string   `json:"feed_name"`
	Tags       []string `json:"tags"`
	Source     string   `json:"source"`
	Status     string   `json:"status"`
	Content    string   `json:"content"`
	Confidence *float32 `json:"confidence,omitempty"`
	Published  *string  `json:"published,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

type DocumentListResponse struct {
	Items   []DocumentItem `json:"items"`
	Total   int            `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
}

type FeedStat struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type DocumentStatsResponse struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
	BySource map[string]int `json:"by_source"`
	Feeds    []FeedStat     `json:"feeds"`
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	// Build WHERE clauses
	var conditions []string
	var args []any
	argIdx := 1

	if feedID := q.Get("feed_id"); feedID != "" {
		conditions = append(conditions, fmt.Sprintf("d.feed_id = $%d", argIdx))
		args = append(args, feedID)
		argIdx++
	}
	if tag := q.Get("tag"); tag != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(d.tags)", argIdx))
		args = append(args, tag)
		argIdx++
	}
	if source := q.Get("source"); source != "" {
		conditions = append(conditions, fmt.Sprintf("d.source = $%d", argIdx))
		args = append(args, source)
		argIdx++
	}
	if status := q.Get("status"); status != "" {
		conditions = append(conditions, fmt.Sprintf("COALESCE(iq.status, 'pending') = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Sort
	sortField := "d.published"
	if s := q.Get("sort"); s == "title" {
		sortField = "d.title"
	} else if s == "created_at" {
		sortField = "d.created_at"
	}
	order := "DESC"
	if o := q.Get("order"); o == "asc" {
		order = "ASC"
	}

	// Count total
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM documents d
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		%s
	`, where)
	var total int
	err := s.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		http.Error(w, "failed to count documents", http.StatusInternalServerError)
		return
	}

	// Fetch page
	listQuery := fmt.Sprintf(`
		SELECT d.id, d.title, d.url, COALESCE(f.name, ''), d.tags, d.source,
		       COALESCE(iq.status, 'pending'), LEFT(d.content, 200), 
		       d.published::text, d.created_at::text
		FROM documents d
		LEFT JOIN feeds f ON f.id = d.feed_id
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, where, sortField, order, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := s.db.Pool.Query(ctx, listQuery, args...)
	if err != nil {
		http.Error(w, "failed to query documents", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []DocumentItem
	for rows.Next() {
		var item DocumentItem
		if err := rows.Scan(&item.ID, &item.Title, &item.URL, &item.FeedName, &item.Tags, &item.Source, &item.Status, &item.Summary, &item.Published, &item.CreatedAt); err != nil {
			http.Error(w, "failed to scan document", http.StatusInternalServerError)
			return
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DocumentListResponse{
		Items:   items,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	})
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var doc DocumentDetail
	err = s.db.Pool.QueryRow(ctx, `
		SELECT d.id, d.title, d.url, COALESCE(f.name, ''), d.tags, d.source,
		       COALESCE(iq.status, 'pending'), d.content, d.confidence, 
		       d.published::text, d.created_at::text
		FROM documents d
		LEFT JOIN feeds f ON f.id = d.feed_id
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		WHERE d.id = $1
	`, id).Scan(&doc.ID, &doc.Title, &doc.URL, &doc.FeedName, &doc.Tags, &doc.Source, &doc.Status, &doc.Content, &doc.Confidence, &doc.Published, &doc.CreatedAt)
	if err != nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleDocumentStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var resp DocumentStatsResponse
	resp.ByStatus = make(map[string]int)
	resp.BySource = make(map[string]int)

	// Total
	if err := s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM documents").Scan(&resp.Total); err != nil {
		http.Error(w, "failed to count documents", http.StatusInternalServerError)
		return
	}

	// By status
	rows, err := s.db.Pool.Query(ctx, `
		SELECT COALESCE(iq.status, 'pending'), COUNT(*)
		FROM documents d
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		GROUP BY 1
	`)
	if err != nil {
		http.Error(w, "failed to query stats", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			http.Error(w, "failed to scan stats", http.StatusInternalServerError)
			return
		}
		resp.ByStatus[status] = count
	}

	// By source
	rows2, err := s.db.Pool.Query(ctx, `
		SELECT source, COUNT(*) FROM documents GROUP BY source
	`)
	if err != nil {
		http.Error(w, "failed to query source stats", http.StatusInternalServerError)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var source string
		var count int
		if err := rows2.Scan(&source, &count); err != nil {
			http.Error(w, "failed to scan source stats", http.StatusInternalServerError)
			return
		}
		resp.BySource[source] = count
	}

	// By feed
	rows3, err := s.db.Pool.Query(ctx, `
		SELECT f.id, f.name, COUNT(d.id)
		FROM feeds f
		LEFT JOIN documents d ON d.feed_id = f.id
		GROUP BY f.id, f.name
		ORDER BY COUNT(d.id) DESC
	`)
	if err != nil {
		http.Error(w, "failed to query feed stats", http.StatusInternalServerError)
		return
	}
	defer rows3.Close()
	for rows3.Next() {
		var fs FeedStat
		if err := rows3.Scan(&fs.ID, &fs.Name, &fs.Count); err != nil {
			http.Error(w, "failed to scan feed stats", http.StatusInternalServerError)
			return
		}
		resp.Feeds = append(resp.Feeds, fs)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleProcessDocuments(w http.ResponseWriter, r *http.Request) {
	if s.onProcess == nil {
		http.Error(w, "process handler not configured", http.StatusInternalServerError)
		return
	}

	if s.processState.Running {
		http.Error(w, "processing already in progress", http.StatusConflict)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.processState.cancel = cancel
	s.processState.Running = true
	s.processState.Total = 0
	s.processState.Completed = 0
	s.processState.Current = "准备中..."

	go func() {
		s.onProcess(ctx)
		s.processState.Running = false
		s.processState.Current = ""
		s.processState.cancel = nil
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "processing"})
}

func (s *Server) handleProcessStop(w http.ResponseWriter, r *http.Request) {
	if !s.processState.Running || s.processState.cancel == nil {
		http.Error(w, "no processing in progress", http.StatusConflict)
		return
	}

	s.processState.cancel()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopping"})
}

func (s *Server) handleProcessStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.processState)
}

// UpdateProcessProgress updates the process progress
func (s *Server) UpdateProcessProgress(total, completed int, current string) {
	s.processState.Total = total
	s.processState.Completed = completed
	s.processState.Current = current
}
