package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-wiki/internal/config"
	"llm-wiki/internal/step1"
	"llm-wiki/internal/step2"
	"llm-wiki/internal/step3"
	"llm-wiki/internal/web"
	"llm-wiki/pkg/database"
	"llm-wiki/pkg/llm"
	vectpkg "llm-wiki/pkg/vector"
)

// App ties together all pipeline components
type App struct {
	cfg       *config.Config
	loader    *config.Loader
	db        *database.DB
	llmClient *llm.Client
	fetcher   *step1.Fetcher
	scheduler *step1.Scheduler
	filter    *step2.Filter
	dedup     *step2.Dedup
	embedder  *vectpkg.Embedder
	writer    *step3.WikiWriter
	ingest    *step3.Ingest
	webServer *web.Server
}

// New creates and wires all components
func New(cfg *config.Config, db *database.DB) *App {
	fetcher := step1.NewFetcher()
	scheduler := step1.NewScheduler(fetcher, cfg.Paths.Raw)

	var llmClient *llm.Client
	embedURL := cfg.Dedup.Vector.EmbeddingURL
	embedKey := cfg.Dedup.Vector.EmbeddingKey
	if embedURL != "" && embedKey != "" {
		llmClient = llm.NewClientWithEmbed(cfg.LLM.APIKey, cfg.LLM.BaseURL, cfg.LLM.Model, embedURL, embedKey)
	} else {
		llmClient = llm.NewClient(cfg.LLM.APIKey, cfg.LLM.BaseURL, cfg.LLM.Model)
	}

	app := &App{
		cfg:       cfg,
		db:        db,
		llmClient: llmClient,
		fetcher:   fetcher,
		scheduler: scheduler,
		filter: step2.NewFilter(
			cfg.Filter.Mode,
			step2.KeywordFilter{
				MatchAny: cfg.Filter.Keyword.MatchAny,
				Tags:     cfg.Filter.Keyword.Tags,
			},
			step2.LLMJudgmentConfig{
				Model:         cfg.Filter.LLMJudgment.Model,
				SampleRate:    cfg.Filter.LLMJudgment.SampleRate,
				MinConfidence: cfg.Filter.LLMJudgment.MinConfidence,
			},
		),
		dedup: step2.NewDedup(
			db.Pool,
			cfg.Dedup.Vector.Threshold,
			cfg.Dedup.Vector.Enabled,
		),
		embedder: vectpkg.NewEmbedder(llmClient, db.Pool, cfg.Dedup.Vector.Model),
		writer:   step3.NewWikiWriter(),
	}

	app.filter.SetLLMClient(llmClient)

	app.ingest = step3.NewIngest(llmClient, app.embedder, app.writer, app.dedup)
	app.webServer = web.NewServer(db, llmClient, cfg)
	app.webServer.SetFetchHandler(func() {
		app.scheduler.RunOnce()
	})
	app.webServer.SetProcessHandler(func() {
		app.processPendingDocuments()
	})
	app.webServer.SetLLMUpdateHandler(func(apiKey, baseURL, model string) {
		if apiKey != "" && baseURL != "" && model != "" {
			app.llmClient = llm.NewClient(apiKey, baseURL, model)
			app.ingest = step3.NewIngest(app.llmClient, app.embedder, app.writer, app.dedup)
			log.Printf("[app] LLM client updated: %s", baseURL)
		}
	})

	// Wire scheduler callbacks: fetch → filter → ingest (direct call)
	scheduler.OnNewItem(func(feedName string, item *step1.Item, filePath string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		app.processOne(ctx, feedName, filePath)
	})

	// Wire progress callback
	scheduler.OnProgress(func(total, completed int, current string) {
		app.webServer.UpdateFetchProgress(total, completed, current)
	})

	scheduler.SetGlobalInterval(cfg.Feeds.Interval)

	// Set default interval if not configured
	if cfg.Feeds.Interval == "" {
		scheduler.SetGlobalInterval("0 */6 * * *")
	}

	// Register feeds from config
	for _, f := range cfg.Feeds.Feeds {
		scheduler.Register(step1.Feed{
			Name: f.Name,
			URL:  f.URL,
			Tags: f.Tags,
		})
	}

	return app
}

func (a *App) processOne(ctx context.Context, feedName, filePath string) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("[pipeline] read %s: %v", filePath, err)
		return
	}

	// Check if URL already exists in database
	url := extractURL(string(content))
	if url != "" {
		var exists bool
		err := a.db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents WHERE url = $1)`, url).Scan(&exists)
		if err == nil && exists {
			log.Printf("[pipeline] skip duplicate: %s", url)
			os.Remove(filePath)
			return
		}
	}

	// Step 2: filter — use feed-level tags (authoritative) + doc tags (supplemental)
	itemTags := a.tagsForFeed(feedName)
	if len(itemTags) == 0 {
		itemTags = []string{} // avoid nil slice in merge
	}
	docTags := step2.ExtractTags(string(content))
	// Merge: feed tags + doc tags, feed tags take priority
	merged := make([]string, 0, len(itemTags)+len(docTags))
	seen := make(map[string]bool)
	for _, t := range itemTags {
		if !seen[t] {
			merged = append(merged, t)
			seen[t] = true
		}
	}
	for _, t := range docTags {
		if !seen[t] {
			merged = append(merged, t)
			seen[t] = true
		}
	}

	// Save URL+hash to PG as raw record (before filter)
	title := extractTitle(string(content))
	contentHash := step2.ComputeHash(title, string(content))
	a.saveDocRecord(ctx, url, contentHash, title, string(content), "raw", feedName)

	decision, err := a.filter.Decide(ctx, merged)
	if err != nil {
		log.Printf("[pipeline] filter %s: %v", filePath, err)
		return
	}

	if decision == step2.Reject {
		log.Printf("[pipeline] rejected: %s", filePath)
		// Move to reject/
		if dest, err := a.moveTo(filePath, "reject"); err == nil {
			log.Printf("[pipeline] moved to %s", dest)
		}
		return
	}

	if decision == step2.Judging {
		log.Printf("[pipeline] needs LLM judgment: %s", filePath)
		// TODO: call LLM for judgment, then proceed or reject
	}

	// Step 3: ingest
	dest, err := a.moveTo(filePath, "cleaned_raw")
	if err != nil {
		log.Printf("[pipeline] move to cleaned: %v", err)
		return
	}

	_, err = a.ingest.Process(ctx, dest)
	if err != nil {
		log.Printf("[pipeline] ingest %s: %v", dest, err)
		return
	}
	log.Printf("[pipeline] ingested: %s", dest)
}

// processPendingDocuments processes all pending documents with LLM
func (a *App) processPendingDocuments() {
	ctx := context.Background()

	// Get total count
	var total int
	err := a.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM documents d
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		WHERE iq.status IS NULL OR iq.status = 'pending'
	`).Scan(&total)
	if err != nil {
		log.Printf("[process] count pending: %v", err)
		return
	}

	a.webServer.UpdateProcessProgress(total, 0, "")

	processed := 0
	batchSize := 100
	for processed < total {
		// Get batch of pending documents
		rows, err := a.db.Pool.Query(ctx, `
			SELECT d.id, d.url, d.title, d.content
			FROM documents d
			LEFT JOIN ingest_queue iq ON iq.document_id = d.id
			WHERE iq.status IS NULL OR iq.status = 'pending'
			ORDER BY d.created_at DESC
			LIMIT $1
		`, batchSize)
		if err != nil {
			log.Printf("[process] query pending: %v", err)
			return
		}

		var docs []struct {
			ID      int64
			URL     string
			Title   string
			Content string
		}
		for rows.Next() {
			var d struct {
				ID      int64
				URL     string
				Title   string
				Content string
			}
			if err := rows.Scan(&d.ID, &d.URL, &d.Title, &d.Content); err != nil {
				continue
			}
			docs = append(docs, d)
		}
		rows.Close()

		if len(docs) == 0 {
			break
		}

		for _, doc := range docs {
			a.webServer.UpdateProcessProgress(total, processed, doc.Title)

			// Save to cleaned_raw for processing
			filePath := filepath.Join(a.cfg.Paths.CleanedRaw, fmt.Sprintf("doc_%d.md", doc.ID))
			if err := os.MkdirAll(a.cfg.Paths.CleanedRaw, 0755); err != nil {
				log.Printf("[process] mkdir: %v", err)
				processed++
				continue
			}
			if err := os.WriteFile(filePath, []byte(doc.Content), 0644); err != nil {
				log.Printf("[process] write file: %v", err)
				processed++
				continue
			}

			// Process with LLM
			_, err := a.ingest.Process(ctx, filePath)
			if err != nil {
				log.Printf("[process] ingest %s: %v", doc.Title, err)
				// Mark as failed
				a.db.Pool.Exec(ctx, `
					INSERT INTO ingest_queue (document_id, status, error)
					VALUES ($1, 'failed', $2)
					ON CONFLICT (document_id) DO UPDATE SET status = 'failed', error = $2
				`, doc.ID, err.Error())
				processed++
				continue
			}

			// Mark as done
			a.db.Pool.Exec(ctx, `
				INSERT INTO ingest_queue (document_id, status, processed_at)
				VALUES ($1, 'done', NOW())
				ON CONFLICT (document_id) DO UPDATE SET status = 'done', processed_at = NOW()
			`, doc.ID)

			processed++
			log.Printf("[process] processed: %s (%d/%d)", doc.Title, processed, total)
		}
	}

	a.webServer.UpdateProcessProgress(total, total, "完成")
}

func (a *App) moveTo(src, targetDir string) (string, error) {
	filename := fmt.Sprintf("%s_%d.md", "src", time.Now().UnixNano())

	// Get base path from config
	var basePath string
	switch targetDir {
	case "cleaned_raw":
		basePath = a.cfg.Paths.CleanedRaw
	case "reject":
		basePath = a.cfg.Paths.Reject
	default:
		basePath = filepath.Join("data", targetDir)
	}

	dest := filepath.Join(basePath, filename)
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return "", err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return "", err
	}
	if err := os.Remove(src); err != nil {
		fmt.Printf("[app] warning: failed to remove source file %s: %v\n", src, err)
	}
	return dest, nil
}

// Start begins the scheduler and web server, then blocks
func (a *App) Start(ctx context.Context) error {
	// Load feeds from database and register to scheduler
	if err := a.loadFeedsFromDB(ctx); err != nil {
		log.Printf("[app] failed to load feeds from DB: %v", err)
	}

	// Start web server in background
	go func() {
		if err := a.webServer.Start(ctx); err != nil {
			log.Printf("[app] web server error: %v", err)
		}
	}()

	// Start scheduler (blocks until ctx.Done)
	a.scheduler.Start(ctx)
	return nil
}

// loadFeedsFromDB loads feeds from database and registers them to scheduler
func (a *App) loadFeedsFromDB(ctx context.Context) error {
	rows, err := a.db.Pool.Query(ctx, `SELECT name, url, tags FROM feeds ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var name, url string
		var tags []string
		if err := rows.Scan(&name, &url, &tags); err != nil {
			log.Printf("[app] scan feed: %v", err)
			continue
		}
		if err := a.scheduler.Register(step1.Feed{
			Name: name,
			URL:  url,
			Tags: tags,
		}); err != nil {
			log.Printf("[app] register feed %s: %v", name, err)
		}
		count++
	}
	log.Printf("[app] loaded %d feeds from database", count)
	return nil
}

// RunOnce triggers one full pipeline cycle
func (a *App) RunOnce() {
	a.scheduler.RunOnce()
	a.writer.Close()
}

// Shutdown gracefully shuts down
func (a *App) Shutdown() {
	a.writer.Close()
}

// DB returns the database handle
func (a *App) DB() *database.DB {
	return a.db
}

// Scheduler returns the scheduler
func (a *App) Scheduler() *step1.Scheduler {
	return a.scheduler
}

// tagsForFeed returns the configured tags for a feed by name
func (a *App) tagsForFeed(feedName string) []string {
	for _, f := range a.cfg.Feeds.Feeds {
		if f.Name == feedName {
			return f.Tags
		}
	}
	return nil
}

func (a *App) saveDocRecord(ctx context.Context, url, contentHash, title, content, source, feedName string) {
	// Find feed_id by name
	var feedID *int64
	if feedName != "" {
		var id int64
		err := a.db.Pool.QueryRow(ctx, `SELECT id FROM feeds WHERE name = $1`, feedName).Scan(&id)
		if err == nil {
			feedID = &id
		}
	}

	// Extract published time
	published := extractPublished(content)

	_, err := a.db.Pool.Exec(ctx, `
		INSERT INTO documents (url, title, content_hash, content, tags, source, file_path, feed_id, published)
		VALUES ($1, $2, $3, $4, $5, $6, '', $7, $8)
		ON CONFLICT (url) DO UPDATE SET source = EXCLUDED.source, title = EXCLUDED.title, feed_id = EXCLUDED.feed_id, published = EXCLUDED.published
	`, url, title, contentHash, content, []string{}, source, feedID, published)
	if err != nil {
		log.Printf("[db] save doc record: %v", err)
	}
}

func extractURL(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "url:") {
			return strings.TrimPrefix(line, "url:")
		}
	}
	return ""
}

func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "title:") {
			return strings.TrimPrefix(line, "title:")
		}
	}
	return ""
}

func extractPublished(content string) *time.Time {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "published:") {
			publishedStr := strings.TrimSpace(strings.TrimPrefix(line, "published:"))
			if t, err := time.Parse(time.RFC3339, publishedStr); err == nil {
				return &t
			}
			// Try other formats
			if t, err := time.Parse("2006-01-02", publishedStr); err == nil {
				return &t
			}
		}
	}
	return nil
}
