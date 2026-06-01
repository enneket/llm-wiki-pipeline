package step3

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WikiWriter handles wiki file writes with a serialised queue
type WikiWriter struct {
	queue chan wikiJob
	wg    sync.WaitGroup
}

type wikiJob struct {
	page    *WikiPage
	jobType string // "write" | "update_index" | "update_log"
}

// WikiPage represents a generated wiki page
type WikiPage struct {
	Title     string
	Slug      string
	Type      string // entity | concept | source
	Tags      []string
	Content   string
	Sources   []string // cleaned_raw paths
	FilePath  string
	WikiLinks []string
}

// NewWikiWriter creates a serialised wiki file writer
func NewWikiWriter() *WikiWriter {
	w := &WikiWriter{
		queue: make(chan wikiJob, 100),
	}
	w.wg.Add(1)
	go w.process()
	return w
}

// Enqueue adds a job to the write queue
func (w *WikiWriter) Enqueue(page *WikiPage, jobType string) error {
	select {
	case w.queue <- wikiJob{page: page, jobType: jobType}:
		return nil
	default:
		return fmt.Errorf("wiki write queue full")
	}
}

// Close waits for all pending writes to complete
func (w *WikiWriter) Close() {
	close(w.queue)
	w.wg.Wait()
}

func (w *WikiWriter) process() {
	defer w.wg.Done()
	for job := range w.queue {
		switch job.jobType {
		case "write":
			w.writePage(job.page)
		case "update_index":
			w.updateIndex()
		case "update_log":
			w.updateLog(job.page)
		}
	}
}

func (w *WikiWriter) writePage(page *WikiPage) error {
	dir := filepath.Join("data", "wiki")
	switch page.Type {
	case "entity":
		dir = filepath.Join(dir, "entities", page.Slug)
	case "concept":
		dir = filepath.Join(dir, "concepts")
	case "source":
		dir = filepath.Join(dir, "sources")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	filename := filepath.Join(dir, page.Slug+".md")
	if page.Type == "source" {
		filename = filepath.Join(dir, fmt.Sprintf("%s_%s.md", page.Slug, time.Now().Format("2006-01-02")))
	}

	content := buildMarkdown(page)
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	page.FilePath = filename
	return nil
}

func (w *WikiWriter) updateIndex() {
	// Walk data/wiki and rebuild index.md
	index := "# 知识库索引\n\n"
	types := []string{"entity", "concept", "source"}

	for _, t := range types {
		dir := filepath.Join("data", "wiki", t+"s")
		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			continue
		}
		index += fmt.Sprintf("## %s\n", capitalize(t))
		for _, e := range entries {
			if e.IsDir() {
				index += fmt.Sprintf("- [[%s]]\n", e.Name())
			} else {
				name := filepath.Base(e.Name())
				name = name[:len(name)-len(".md")]
				index += fmt.Sprintf("- [[%s]]\n", name)
			}
		}
		index += "\n"
	}

	idxPath := filepath.Join("data", "wiki", "index.md")
	if err := os.WriteFile(idxPath, []byte(index), 0644); err != nil {
		fmt.Printf("[writer] failed to update index: %v\n", err)
	}
}

func (w *WikiWriter) updateLog(page *WikiPage) {
	logLine := fmt.Sprintf("%s Ingest %s (%s)\n", time.Now().Format("2006-01-02 15:04"), page.Title, page.Type)
	logPath := filepath.Join("data", "wiki", "log.md")

	// Append to log
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("[writer] failed to open log: %v\n", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(logLine); err != nil {
		fmt.Printf("[writer] failed to write log: %v\n", err)
	}
}

func buildMarkdown(page *WikiPage) string {
	// YAML frontmatter
	frontmatter := fmt.Sprintf(`---
title: %s
type: %s
tags: [%s]
sources: [%s]
created: %s
last_modified: %s
---
`, page.Title, page.Type, joinTags(page.Tags), joinSources(page.Sources),
		time.Now().Format("2006-01-02"), time.Now().Format("2006-01-02"))

	// WikiLink references
	wikilinks := ""
	for _, wl := range page.WikiLinks {
		wikilinks += fmt.Sprintf("[[%s]] ", wl)
	}
	if wikilinks != "" {
		wikilinks = "\n> Links: " + wikilinks + "\n"
	}

	return fmt.Sprintf("%s# %s%s\n\n%s", frontmatter, page.Title, wikilinks, page.Content)
}

func joinTags(tags []string) string {
	result := ""
	for i, t := range tags {
		if i > 0 {
			result += ", "
		}
		result += t
	}
	return result
}

func joinSources(sources []string) string {
	result := ""
	for i, s := range sources {
		if i > 0 {
			result += ", "
		}
		result += `"` + s + `"`
	}
	return result
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}
