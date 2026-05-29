package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"llm-wiki/internal/config"
	"llm-wiki/pkg/database"
	"llm-wiki/pkg/llm"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	db   *database.DB
	llm  *llm.Client
	port string
	cfg  *config.Config
}

func NewServer(db *database.DB, llmClient *llm.Client) *Server {
	return &Server{
		db:   db,
		llm:  llmClient,
		port: "6006",
	}
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("POST /api/query", s.handleQuery)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/feeds", s.handleListFeeds)
	mux.HandleFunc("POST /api/feeds", s.handleAddFeed)
	mux.HandleFunc("DELETE /api/feeds/{id}", s.handleDeleteFeed)
	mux.HandleFunc("GET /api/feeds/export", s.handleExportFeeds)
	mux.HandleFunc("POST /api/feeds/import", s.handleImportFeeds)
	mux.HandleFunc("GET /api/wiki", s.handleListWiki)
	mux.HandleFunc("GET /api/wiki/{slug}", s.handleGetWiki)
	mux.HandleFunc("GET /api/documents", s.handleListDocuments)
	mux.HandleFunc("GET /api/documents/stats", s.handleDocumentStats)
	mux.HandleFunc("GET /api/documents/{id}", s.handleGetDocument)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("GET /api/settings/{category}", s.handleGetSettingCategory)
	mux.HandleFunc("PUT /api/settings/{category}", s.handleUpdateSettingCategory)

	// Static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("embed static files: %w", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(staticFS)))

	server := &http.Server{
		Addr:         ":" + s.port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("[web] listening on :%s", s.port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[web] server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Println("[web] shutting down...")
	return server.Shutdown(shutdownCtx)
}
