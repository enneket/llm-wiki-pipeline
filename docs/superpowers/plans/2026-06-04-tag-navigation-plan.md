# Tag Navigation Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate category navigation pages from frontmatter tags so users can browse wiki content by domain/topic.

**Architecture:** Improve LLM analysis to return domain-level categories, use those as tags for all generated pages, then scan all wiki pages to auto-generate category index pages under `data/wiki/categories/`.

**Tech Stack:** Go, YAML frontmatter parsing

---

### Task 1: Add Categories to analysisResult and analyze prompt

**Files:**
- Modify: `internal/step3/cot.go:14-19` (analysisResult struct)
- Modify: `internal/step3/cot.go:32-48` (analyze prompt)
- Modify: `internal/step3/cot.go:126-151` (parseJSON)

- [ ] **Step 1: Add Categories field to analysisResult**

```go
type analysisResult struct {
	Entities        []string // key entities
	Concepts        []string // key concepts
	Categories      []string // domain-level tags (e.g. "AI", "security", "programming")
	LinkSuggestions []string // suggested wikilinks (existing pages)
	Summary         string   // concise summary of the document
}
```

- [ ] **Step 2: Update analyze prompt to request categories**

Change the prompt JSON example from:
```json
{
  "entities": ["entity1", "entity2"],
  "concepts": ["concept1", "concept2"],
  "link_suggestions": ["ExistingPage1", "ExistingPage2"],
  "summary": "2-3 sentence summary of this document"
}
```
to:
```json
{
  "entities": ["entity1", "entity2"],
  "concepts": ["concept1", "concept2"],
  "categories": ["AI", "security"],
  "link_suggestions": ["ExistingPage1", "ExistingPage2"],
  "summary": "2-3 sentence summary of this document"
}
```

Add instruction: `categories should be broad domain tags like "AI", "security", "programming", "business", "science", "hardware", "open_source", etc. Use English, lowercase with underscores.`

- [ ] **Step 3: Update parseJSON to parse categories**

Add `Categories []string \`json:"categories"\`` to the anonymous struct in parseJSON, and copy to `out.Categories`.

- [ ] **Step 4: Run go build to verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/step3/cot.go
git commit -m "feat: add categories to LLM analysis result"
```

---

### Task 2: Use categories as tags in ingest flow

**Files:**
- Modify: `internal/step3/ingest.go:62-102` (Process method)

- [ ] **Step 1: Set source page tags from analysis.Categories**

Change line 62 area. After `page.Sources = []string{filePath}`, add:
```go
if len(analysis.Categories) > 0 {
    page.Tags = analysis.Categories
}
```

- [ ] **Step 2: Set entity page tags from analysis.Categories**

Change the entity page creation (line 74-78) from:
```go
Tags:      []string{entity},
```
to:
```go
Tags:      analysis.Categories,
```

- [ ] **Step 3: Set concept page tags from analysis.Categories**

Change the concept page creation (line 90-94) from:
```go
Tags:      []string{concept},
```
to:
```go
Tags:      analysis.Categories,
```

- [ ] **Step 4: Enqueue category update after all writes**

After the concept generation loop (after line 102), add:
```go
if err := i.writer.Enqueue(page, "update_categories"); err != nil {
    return nil, fmt.Errorf("enqueue categories: %w", err)
}
```

- [ ] **Step 5: Run go build to verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/step3/ingest.go
git commit -m "feat: use analysis categories as tags for wiki pages"
```

---

### Task 3: Implement category page generation in writer

**Files:**
- Modify: `internal/step3/writer.go:22-25` (wikiJob type comment)
- Modify: `internal/step3/writer.go:66-78` (process method)
- Create: `internal/wiki/categories.go`

- [ ] **Step 1: Add "update_categories" to process switch**

In `writer.go` process method, add a new case:
```go
case "update_categories":
    w.updateCategories()
```

- [ ] **Step 2: Create categories.go with page scanning and generation logic**

```go
package wiki

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CategoryInfo holds pages belonging to a category
type CategoryInfo struct {
	Name     string
	Entities []string
	Concepts []string
	Sources  []string
}

// ScanWikiPages walks data/wiki and extracts tags from frontmatter
func ScanWikiPages() map[string]*CategoryInfo {
	categories := make(map[string]*CategoryInfo)

	dirs := map[string]string{
		"entity":  filepath.Join("data", "wiki", "entities"),
		"concept": filepath.Join("data", "wiki", "concepts"),
		"source":  filepath.Join("data", "wiki", "sources"),
	}

	for pageType, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			var filePath string
			if e.IsDir() {
				// entity pages are in subdirectories
				mdFile := e.Name() + ".md"
				filePath = filepath.Join(dir, e.Name(), mdFile)
			} else {
				if !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				filePath = filepath.Join(dir, e.Name())
			}

			tags, title := parseFrontmatterTags(filePath)
			if title == "" {
				if e.IsDir() {
					title = e.Name()
				} else {
					title = strings.TrimSuffix(e.Name(), ".md")
				}
			}

			pageName := title
			if pageName == "" {
				continue
			}

			for _, tag := range tags {
				tag = strings.TrimSpace(tag)
				if tag == "" {
					continue
				}
				if categories[tag] == nil {
					categories[tag] = &CategoryInfo{Name: tag}
				}
				switch pageType {
				case "entity":
					categories[tag].Entities = append(categories[tag].Entities, pageName)
				case "concept":
					categories[tag].Concepts = append(categories[tag].Concepts, pageName)
				case "source":
					categories[tag].Sources = append(categories[tag].Sources, pageName)
				}
			}
		}
	}

	return categories
}

// parseFrontmatterTags extracts tags and title from YAML frontmatter
func parseFrontmatterTags(filePath string) (tags []string, title string) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if lineNum == 1 && line == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter && line == "---" {
			break
		}
		if inFrontmatter {
			if strings.HasPrefix(line, "tags:") {
				val := strings.TrimPrefix(line, "tags:")
				val = strings.TrimSpace(val)
				// Parse [tag1, tag2] format
				val = strings.Trim(val, "[]")
				if val != "" {
					parts := strings.Split(val, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							tags = append(tags, p)
						}
					}
				}
			}
			if strings.HasPrefix(line, "title:") {
				title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
				// Remove quotes if present
				title = strings.Trim(title, "\"'")
			}
		}
	}

	return tags, title
}

// GenerateCategoryPage writes a category markdown file
func GenerateCategoryPage(cat *CategoryInfo) string {
	sort.Strings(cat.Entities)
	sort.Strings(cat.Concepts)
	sort.Strings(cat.Sources)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("---\ntitle: \"%s\"\ntype: category\n---\n# %s\n\n", cat.Name, cat.Name))

	if len(cat.Entities) > 0 {
		b.WriteString("## Entities\n")
		for _, e := range cat.Entities {
			b.WriteString(fmt.Sprintf("- [[%s]]\n", e))
		}
		b.WriteString("\n")
	}

	if len(cat.Concepts) > 0 {
		b.WriteString("## Concepts\n")
		for _, c := range cat.Concepts {
			b.WriteString(fmt.Sprintf("- [[%s]]\n", c))
		}
		b.WriteString("\n")
	}

	if len(cat.Sources) > 0 {
		b.WriteString("## Sources\n")
		for _, s := range cat.Sources {
			b.WriteString(fmt.Sprintf("- [[%s]]\n", s))
		}
		b.WriteString("\n")
	}

	return b.String()
}
```

- [ ] **Step 3: Add updateCategories method to WikiWriter**

In `writer.go`, add:
```go
func (w *WikiWriter) updateCategories() {
	categories := wiki.ScanWikiPages()

	catDir := filepath.Join("data", "wiki", "categories")
	if err := os.MkdirAll(catDir, 0755); err != nil {
		fmt.Printf("[writer] failed to create categories dir: %v\n", err)
		return
	}

	// Clean old category files
	oldFiles, _ := os.ReadDir(catDir)
	for _, f := range oldFiles {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		os.Remove(filepath.Join(catDir, f.Name()))
	}

	// Write new category pages
	for _, cat := range categories {
		content := wiki.GenerateCategoryPage(cat)
		filename := filepath.Join(catDir, slugify(cat.Name)+".md")
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			fmt.Printf("[writer] failed to write category %s: %v\n", cat.Name, err)
		}
	}
}
```

Add import for `"strings"` and `"llm-wiki/internal/wiki"` if needed, or inline the logic.

- [ ] **Step 4: Run go build to verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/step3/writer.go internal/wiki/categories.go
git commit -m "feat: auto-generate category navigation pages from tags"
```

---

### Task 4: Verify end-to-end

- [ ] **Step 1: Run go build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: PASS

- [ ] **Step 3: Verify existing tests still pass**

Run: `go test ./...`
Expected: PASS (or no test files)

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: address review feedback"
```
