package web

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"llm-wiki/pkg/feedutil"
)

type Feed struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Tags      []string `json:"tags"`
	Interval  string   `json:"interval"`
	LastFetch *string  `json:"last_fetch,omitempty"`
}

func (s *Server) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, name, url, tags, interval, last_fetch::text
		FROM feeds
		ORDER BY name
	`)
	if err != nil {
		http.Error(w, "failed to query feeds", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		var lastFetch *string
		if err := rows.Scan(&f.ID, &f.Name, &f.URL, &f.Tags, &f.Interval, &lastFetch); err != nil {
			http.Error(w, "failed to scan feed", http.StatusInternalServerError)
			return
		}
		f.LastFetch = lastFetch
		feeds = append(feeds, f)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(feeds)
}

type AddFeedRequest struct {
	Name     string   `json:"name"`
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
	Interval string   `json:"interval"`
}

func (s *Server) handleAddFeed(w http.ResponseWriter, r *http.Request) {
	var req AddFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.URL == "" {
		http.Error(w, "name and url are required", http.StatusBadRequest)
		return
	}

	if req.Interval == "" {
		req.Interval = "0 */6 * * *"
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}

	ctx := r.Context()
	_, err := s.db.Pool.Exec(ctx, `
		INSERT INTO feeds (name, url, tags, interval)
		VALUES ($1, $2, $3, $4)
	`, req.Name, req.URL, req.Tags, req.Interval)
	if err != nil {
		http.Error(w, "failed to add feed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func (s *Server) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid feed id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	result, err := s.db.Pool.Exec(ctx, "DELETE FROM feeds WHERE id = $1", id)
	if err != nil {
		http.Error(w, "failed to delete feed", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "feed not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (s *Server) handleUpdateFeed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid feed id", http.StatusBadRequest)
		return
	}

	var req AddFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.URL == "" {
		http.Error(w, "name and url are required", http.StatusBadRequest)
		return
	}

	if req.Interval == "" {
		req.Interval = "0 */6 * * *"
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}

	ctx := r.Context()
	result, err := s.db.Pool.Exec(ctx, `
		UPDATE feeds SET name = $1, url = $2, tags = $3, interval = $4, updated_at = NOW()
		WHERE id = $5
	`, req.Name, req.URL, req.Tags, req.Interval, id)
	if err != nil {
		http.Error(w, "failed to update feed", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "feed not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func (s *Server) handleFetchFeeds(w http.ResponseWriter, r *http.Request) {
	if s.onFetch == nil {
		http.Error(w, "fetch handler not configured", http.StatusInternalServerError)
		return
	}

	go s.onFetch()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "fetching"})
}

func (s *Server) handleExportFeeds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool.Query(ctx, `
		SELECT name, url, tags
		FROM feeds
		ORDER BY name
	`)
	if err != nil {
		http.Error(w, "failed to query feeds", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var feeds []ExportFeed
	for rows.Next() {
		var f ExportFeed
		if err := rows.Scan(&f.Name, &f.URL, &f.Tags); err != nil {
			http.Error(w, "failed to scan feed", http.StatusInternalServerError)
			return
		}
		feeds = append(feeds, f)
	}

	format := r.URL.Query().Get("format")
	if format == "opml" {
		exportOPML(w, feeds)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=feeds.json")
		json.NewEncoder(w).Encode(feeds)
	}
}

type ExportFeed struct {
	Name string   `json:"name" xml:"-"`
	URL  string   `json:"url" xml:"-"`
	Tags []string `json:"tags" xml:"-"`
}

func exportOPML(w http.ResponseWriter, feeds []ExportFeed) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=feeds.opml")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>LLM Wiki Feeds</title></head>
  <body>
`)
	for _, f := range feeds {
		fmt.Fprintf(w, `    <outline type="rss" text="%s" title="%s" xmlUrl="%s"/>
`, xmlEscape(f.Name), xmlEscape(f.Name), xmlEscape(f.URL))
	}
	fmt.Fprint(w, `  </body>
</opml>
`)
}

func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (s *Server) handleImportFeeds(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read file", http.StatusInternalServerError)
		return
	}

	var parsed []feedutil.FeedEntry
	if feedutil.DetectFormat(data) == "opml" {
		parsed, err = feedutil.ParseOPML(data)
	} else {
		parsed, err = feedutil.ParseURLList(data)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("parse error: %v", err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	added := 0
	for _, f := range parsed {
		if f.URL == "" {
			continue
		}
		f.URL = strings.TrimSpace(f.URL)
		var exists bool
		err := s.db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM feeds WHERE url = $1)", f.URL).Scan(&exists)
		if err != nil {
			log.Printf("[import] check exists for %q: %v", f.URL, err)
			continue
		}
		if exists {
			continue
		}
		if f.Tags == nil {
			f.Tags = []string{}
		}
		// Deduplicate name by appending numeric suffix
		name := f.Name
		for i := 2; ; i++ {
			var nameTaken bool
			err := s.db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM feeds WHERE name = $1)", name).Scan(&nameTaken)
			if err != nil || !nameTaken {
				break
			}
			name = fmt.Sprintf("%s_%d", f.Name, i)
		}
		_, err = s.db.Pool.Exec(ctx, `
			INSERT INTO feeds (name, url, tags, interval)
			VALUES ($1, $2, $3, '0 */6 * * *')
		`, name, f.URL, f.Tags)
		if err != nil {
			log.Printf("[import] insert feed %q (%s): %v", name, f.URL, err)
			continue
		}
		added++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"added": added, "total": len(parsed)})
}
