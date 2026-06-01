package step3

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"llm-wiki/pkg/llm"
	vectpkg "llm-wiki/pkg/vector"
)

// analysisResult is the output of Step 1 (Analysis) of the CoT ingest
type analysisResult struct {
	Entities        []string // key entities
	Concepts        []string // key concepts
	LinkSuggestions []string // suggested wikilinks (existing pages)
	Summary         string   // concise summary of the document
}

// analyze calls LLM to produce a structured analysis of the source document
func (i *Ingest) analyze(ctx context.Context, title, content string, preResults []vectpkg.PreSearchResult) (*analysisResult, error) {
	// Build context from pre-search results (related wiki pages)
	contextLines := ""
	for _, r := range preResults {
		contextLines += "- " + r.Title + ": " + truncate(r.Content, 200) + "\n"
	}
	if contextLines == "" {
		contextLines = "(no related wiki pages found)"
	}

	prompt := `You are analyzing a web document for a personal wiki.
Document title: ` + title + `
---
Document content (truncated):
` + truncate(content, 3000) + `
---
Related existing wiki pages:
` + contextLines + `

Respond with a JSON object with exactly these fields:
{
  "entities": ["entity1", "entity2"],
  "concepts": ["concept1", "concept2"],
  "link_suggestions": ["ExistingPage1", "ExistingPage2"],
  "summary": "2-3 sentence summary of this document"
}
Only output valid JSON, no markdown wrapping.`

	resp, err := i.llmClient.Complete(ctx, []llm.ChatMessage{
		{Role: "system", Content: "You extract structured information from web documents."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return &analysisResult{Summary: truncate(content, 500)}, err
	}

	result := &analysisResult{}
	if err := parseJSON(resp, result); err != nil {
		// Fallback: return truncated content as summary
		return &analysisResult{Summary: truncate(content, 500)}, nil
	}
	return result, nil
}

// generate calls LLM to produce the actual wiki page content
func (i *Ingest) generate(ctx context.Context, title string, analysis *analysisResult, preResults []vectpkg.PreSearchResult) (*WikiPage, error) {
	contextLines := ""
	for _, r := range preResults {
		contextLines += "- [[" + r.Slug + "]] (" + r.Title + ")\n"
	}
	if contextLines == "" {
		contextLines = "(no related pages)"
	}

	prompt := `You are writing a wiki page for a personal knowledge base.

Title: ` + title + `
Analysis:
- Entities: ` + strings.Join(analysis.Entities, ", ") + `
- Concepts: ` + strings.Join(analysis.Concepts, ", ") + `
- Related pages:
` + contextLines + `
Summary: ` + analysis.Summary + `

Write a concise, informative wiki page. Use [[wikilink]] syntax for references to other pages.
Format:
---
title: ` + title + `
type: source
tags: []
sources: []
created: ` + time.Now().Format("2006-01-02") + `
last_modified: ` + time.Now().Format("2006-01-02") + `
---
# ` + title + `

[body with proper wikilinks to related concepts and entities]`

	resp, err := i.llmClient.Complete(ctx, []llm.ChatMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, err
	}

	page := &WikiPage{
		Title:     title,
		Slug:      slugify(title),
		Type:      "source",
		Tags:      analysis.Entities,
		Content:   resp,
		WikiLinks: analysis.LinkSuggestions,
	}
	return page, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// parseJSON parses JSON response from LLM into analysisResult
func parseJSON(s string, out *analysisResult) error {
	// Try to find JSON object braces
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return nil
	}
	inner := s[start : end+1]

	// Use standard json library
	var result struct {
		Entities        []string `json:"entities"`
		Concepts        []string `json:"concepts"`
		LinkSuggestions []string `json:"link_suggestions"`
		Summary         string   `json:"summary"`
	}

	if err := json.Unmarshal([]byte(inner), &result); err != nil {
		return err
	}

	out.Entities = result.Entities
	out.Concepts = result.Concepts
	out.LinkSuggestions = result.LinkSuggestions
	out.Summary = result.Summary
	return nil
}
