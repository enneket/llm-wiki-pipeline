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

	// Wire scheduler callbacks: fetch → filter → ingest (direct call)
	scheduler.OnNewItem(func(feedName string, item *step1.Item, filePath string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		app.processOne(ctx, feedName, filePath)
	})

	scheduler.SetGlobalInterval(cfg.Feeds.Interval)

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
	url := extractURL(string(content))
	contentHash := step2.ComputeHash(extractTitle(string(content)), string(content))
	a.saveDocRecord(ctx, url, contentHash, "raw", feedName)

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

func (a *App) saveDocRecord(ctx context.Context, url, contentHash, source, feedName string) {
	_, err := a.db.Pool.Exec(ctx, `
		INSERT INTO documents (url, title, content_hash, content, tags, source, file_path)
		VALUES ($1, '', $2, '', $3, $4, '')
		ON CONFLICT (url) DO UPDATE SET source = EXCLUDED.source
	`, url, contentHash, []string{}, source)
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
