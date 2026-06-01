package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"llm-wiki/internal/config"
	"llm-wiki/pkg/database"
	"llm-wiki/pkg/llm"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	db       *database.DB
	llm      *llm.Client
	port     string
	cfg      *config.Config
	apiToken string
}

func NewServer(db *database.DB, llmClient *llm.Client, cfg *config.Config) *Server {
	port := cfg.Web.Port
	if port == "" {
		port = "6006"
	}
	return &Server{
		db:       db,
		llm:      llmClient,
		port:     port,
		cfg:      cfg,
		apiToken: cfg.Web.APIToken,
	}
}

// authMiddleware validates Bearer token if apiToken is configured
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for static files and if no token configured
		if s.apiToken == "" || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		if parts[1] != s.apiToken {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
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

	// Apply auth middleware
	handler := s.authMiddleware(mux)

	server := &http.Server{
		Addr:         ":" + s.port,
		Handler:      handler,
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
