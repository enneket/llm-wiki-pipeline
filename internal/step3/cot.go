package step3

import (
	"context"
	"strings"

	"llm-wiki/pkg/llm"
	vectpkg "llm-wiki/pkg/vector"
)

// analysisResult is the output of Step 1 (Analysis) of the CoT ingest
type analysisResult struct {
	Entities        []string   // key entities
	Concepts        []string   // key concepts
	LinkSuggestions []string   // suggested wikilinks (existing pages)
	Summary         string     // concise summary of the document
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
created: 2026-05-19
last_modified: 2026-05-19
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

// simple JSON parser for the analysis response
func parseJSON(s string, out interface{}) error {
	// Try to find JSON object braces
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		// Try array
		start = strings.Index(s, "[")
		end = strings.LastIndex(s, "]")
		if start == -1 || end == -1 || end <= start {
			return nil // Try anyway
		}
	}
	inner := s[start : end+1]

	// Manual parse for the specific struct
	result := &struct {
		Entities        []string `json:"entities"`
		Concepts        []string `json:"concepts"`
		LinkSuggestions []string `json:"link_suggestions"`
		Summary         string   `json:"summary"`
	}{}

	// Simple string extraction
	if v := extractJSON("entities", inner); v != "" {
		for _, e := range strings.Split(v, ",") {
			e = strings.TrimSpace(strings.Trim(e, "[]\" "))
			if e != "" {
				result.Entities = append(result.Entities, e)
			}
		}
	}
	if v := extractJSON("concepts", inner); v != "" {
		for _, e := range strings.Split(v, ",") {
			e = strings.TrimSpace(strings.Trim(e, "[]\" "))
			if e != "" {
				result.Concepts = append(result.Concepts, e)
			}
		}
	}
	if v := extractJSON("link_suggestions", inner); v != "" {
		for _, e := range strings.Split(v, ",") {
			e = strings.TrimSpace(strings.Trim(e, "[]\" "))
			if e != "" {
				result.LinkSuggestions = append(result.LinkSuggestions, e)
			}
		}
	}
	if v := extractJSON("summary", inner); v != "" {
		result.Summary = v
	}

	// Copy to output via reflection-free approach
	if ae, ok := out.(*analysisResult); ok {
		ae.Entities = result.Entities
		ae.Concepts = result.Concepts
		ae.LinkSuggestions = result.LinkSuggestions
		ae.Summary = result.Summary
	}
	return nil
}

func extractJSON(key, body string) string {
	// Find "key": "value" or "key": ["items"]
	pattern := `"` + key + `":`
	idx := strings.Index(body, pattern)
	if idx == -1 {
		return ""
	}
	rest := body[idx+len(pattern):]
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 {
		return ""
	}
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end == -1 {
			return ""
		}
		return rest[:end+1]
	}
	if strings.HasPrefix(rest, `"`) {
		rest = rest[1:]
		end := strings.Index(rest, `"`)
		if end == -1 {
			return ""
		}
		return rest[:end]
	}
	return ""
}