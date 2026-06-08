package step3

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"llm-wiki/internal/step2"
	"llm-wiki/pkg/llm"
	vectpkg "llm-wiki/pkg/vector"
)

// Ingest processes a document through the full ingest pipeline
type Ingest struct {
	llmClient        *llm.Client
	embedder         *vectpkg.Embedder
	writer           *WikiWriter
	dedup            *step2.Dedup
	pool             *pgxpool.Pool
	filterTags       []string
	embeddingContext bool // Use embedding search for wikilink context
}

// NewIngest creates a new ingest pipeline
func NewIngest(llmClient *llm.Client, embedder *vectpkg.Embedder, writer *WikiWriter, dedup *step2.Dedup, pool *pgxpool.Pool, filterTags []string, embeddingContext bool) *Ingest {
	return &Ingest{
		llmClient:        llmClient,
		embedder:         embedder,
		writer:           writer,
		dedup:            dedup,
		pool:             pool,
		filterTags:       filterTags,
		embeddingContext: embeddingContext,
	}
}

// Process ingests a single document file
func (i *Ingest) Process(ctx context.Context, filePath string) (*WikiPage, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	title := step2.ParseFrontmatter(string(content))
	if title == "" {
		title = filepath.Base(filePath)
	}

	// Step 1: Get existing pages for wikilink context
	var preResults []vectpkg.PreSearchResult
	if i.embeddingContext && i.embedder != nil {
		// Use embedding search for better context
		preResults, _ = i.embedder.SearchWiki(ctx, string(content), 5)
	} else {
		// Use database query (no embedding API call)
		preResults, _ = i.getExistingPages(ctx, 50)
	}

	// Step 2: Two-Step CoT Ingest
	analysis, err := i.analyze(ctx, title, string(content), preResults)
	if err != nil {
		return nil, fmt.Errorf("analysis step: %w", err)
	}

	// Step 3: Generate wiki page
	page, err := i.generate(ctx, title, analysis, preResults)
	if err != nil {
		return nil, fmt.Errorf("generate step: %w", err)
	}

	page.Sources = []string{filePath}

	if len(analysis.Categories) > 0 {
		page.Tags = analysis.Categories
	}

	// Enqueue source page write
	if err := i.writer.Enqueue(page, "write"); err != nil {
		return nil, fmt.Errorf("enqueue write: %w", err)
	}
	if err := i.writer.Enqueue(page, "update_log"); err != nil {
		return nil, fmt.Errorf("enqueue log: %w", err)
	}

	// Generate entity pages from analysis results
	for _, entity := range analysis.Entities {
		entityPage := &WikiPage{
			Title:     entity,
			Slug:      slugify(entity),
			Type:      "entity",
			Tags:      analysis.Categories,
			Content:   fmt.Sprintf("Entity: %s\n\nRelated to: [[%s]]", entity, page.Title),
			Sources:   []string{filePath},
			WikiLinks: []string{page.Title},
		}
		if err := i.writer.Enqueue(entityPage, "write"); err != nil {
			return nil, fmt.Errorf("enqueue entity write: %w", err)
		}
	}

	// Generate concept pages from analysis results
	for _, concept := range analysis.Concepts {
		conceptPage := &WikiPage{
			Title:     concept,
			Slug:      slugify(concept),
			Type:      "concept",
			Tags:      analysis.Categories,
			Content:   fmt.Sprintf("Concept: %s\n\nDiscussed in: [[%s]]", concept, page.Title),
			Sources:   []string{filePath},
			WikiLinks: []string{page.Title},
		}
		if err := i.writer.Enqueue(conceptPage, "write"); err != nil {
			return nil, fmt.Errorf("enqueue concept write: %w", err)
		}
	}

	if err := i.writer.Enqueue(page, "update_categories"); err != nil {
		return nil, fmt.Errorf("enqueue categories: %w", err)
	}

	// Store suggested tags (categories not in filter tags)
	if len(analysis.Categories) > 0 {
		i.storeSuggestedTags(ctx, analysis.Categories)
	}

	return page, nil
}

func slugify(name string) string {
	result := ""
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			result += string(r + 'a' - 'A')
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result += string(r)
		} else if r == ' ' || r == '-' || r == '_' {
			result += "_"
		}
	}
	return result
}

// storeSuggestedTags stores categories that are not in filter tags as suggestions
func (i *Ingest) storeSuggestedTags(ctx context.Context, categories []string) {
	if i.pool == nil || len(i.filterTags) == 0 {
		return
	}

	// Build a set of filter tags for quick lookup
	filterSet := make(map[string]bool)
	for _, tag := range i.filterTags {
		filterSet[strings.ToLower(tag)] = true
	}

	for _, cat := range categories {
		catLower := strings.ToLower(cat)
		// Skip if already in filter tags
		if filterSet[catLower] {
			continue
		}

		// Insert or update suggested tag
		_, err := i.pool.Exec(ctx, `
			INSERT INTO suggested_tags (tag, source_count, first_seen, last_seen, status)
			VALUES ($1, 1, NOW(), NOW(), 'pending')
			ON CONFLICT (tag) DO UPDATE SET 
				source_count = suggested_tags.source_count + 1,
				last_seen = NOW()
		`, cat)
		if err != nil {
			log.Printf("[ingest] failed to store suggested tag %s: %v", cat, err)
		}
	}
}

// getExistingPages gets existing wiki page titles from DB for wikilink context
func (i *Ingest) getExistingPages(ctx context.Context, limit int) ([]vectpkg.PreSearchResult, error) {
	if i.writer.pool == nil {
		return nil, nil
	}

	rows, err := i.writer.pool.Query(ctx, `
		SELECT title, slug, LEFT(content, 200)
		FROM wiki_pages
		WHERE page_type IN ('entity', 'concept')
		ORDER BY last_modified DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []vectpkg.PreSearchResult
	for rows.Next() {
		var r vectpkg.PreSearchResult
		if err := rows.Scan(&r.Title, &r.Slug, &r.Content); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}
