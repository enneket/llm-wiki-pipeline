package web

import (
	"encoding/json"
	"net/http"
)

type WikiPage struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	PageType     string   `json:"page_type"`
	Tags         []string `json:"tags"`
	Content      string   `json:"content"`
	CreatedAt    string   `json:"created_at"`
	LastModified string   `json:"last_modified"`
}

type WikiSummary struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	Slug      string   `json:"slug"`
	PageType  string   `json:"page_type"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

func (s *Server) handleListWiki(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, title, slug, page_type, tags, created_at::text
		FROM wiki_pages
		ORDER BY last_modified DESC
		LIMIT 100
	`)
	if err != nil {
		http.Error(w, "failed to query wiki pages", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var pages []WikiSummary
	for rows.Next() {
		var p WikiSummary
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.PageType, &p.Tags, &p.CreatedAt); err != nil {
			http.Error(w, "failed to scan wiki page", http.StatusInternalServerError)
			return
		}
		pages = append(pages, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pages)
}

func (s *Server) handleGetWiki(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var page WikiPage
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, title, slug, page_type, tags, content, created_at::text, last_modified::text
		FROM wiki_pages
		WHERE slug = $1
	`, slug).Scan(&page.ID, &page.Title, &page.Slug, &page.PageType, &page.Tags, &page.Content, &page.CreatedAt, &page.LastModified)
	if err != nil {
		http.Error(w, "wiki page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
