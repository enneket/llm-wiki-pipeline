package step3

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"llm-wiki/internal/step2"
	"llm-wiki/pkg/llm"
	vectpkg "llm-wiki/pkg/vector"
)

// Ingest processes a document through the full ingest pipeline
type Ingest struct {
	llmClient *llm.Client
	embedder  *vectpkg.Embedder
	writer    *WikiWriter
	dedup     *step2.Dedup
}

// NewIngest creates a new ingest pipeline
func NewIngest(llmClient *llm.Client, embedder *vectpkg.Embedder, writer *WikiWriter, dedup *step2.Dedup) *Ingest {
	return &Ingest{
		llmClient: llmClient,
		embedder:  embedder,
		writer:    writer,
		dedup:     dedup,
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

	// Step 1: Embedding pre-search for wikilink context
	preResults, err := i.embedder.SearchWiki(ctx, string(content), 5)
	if err != nil {
		preResults = nil // Non-fatal: continue without pre-search context
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
