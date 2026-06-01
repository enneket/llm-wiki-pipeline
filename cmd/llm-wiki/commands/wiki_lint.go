package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var wikiLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Check wiki consistency and report issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		return lintWiki()
	},
}

func lintWiki() error {
	issues := 0
	wikiDir := "data/wiki"

	// Collect all known wiki pages
	knownPages := make(map[string]bool)
	for _, t := range []string{"entities", "concepts", "sources"} {
		dir := filepath.Join(wikiDir, t)
		if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && t == "entities" {
				// entities/ is flat under entities/<slug>/
				if path != dir {
					name := filepath.Base(path)
					knownPages[strings.ToLower(name)] = true
				}
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				name := strings.TrimSuffix(filepath.Base(path), ".md")
				// strip date suffix for sources: title_2026-01-02.md
				if t == "sources" {
					parts := strings.Split(name, "_")
					if len(parts) > 1 && len(parts[len(parts)-1]) == 10 {
						name = strings.Join(parts[:len(parts)-1], "_")
					}
				}
				knownPages[strings.ToLower(name)] = true
			}
			return nil
		}); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("walk %s: %w", t, err)
		}
	}

	// Check all wiki files for broken wikilinks
	linkRE := regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	for _, t := range []string{"entities", "concepts", "sources"} {
		dir := filepath.Join(wikiDir, t)
		if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			content := string(data)
			title := extractFrontmatterTitle(content)
			if title == "" {
				title = filepath.Base(path)
			}
			for _, m := range linkRE.FindAllStringSubmatch(content, -1) {
				linked := strings.ToLower(m[1])
				if !knownPages[linked] && !strings.HasPrefix(linked, "http") {
					fmt.Printf("[WARN] %s: broken wikilink [[%s]]\n", title, m[1])
					issues++
				}
			}
			return nil
		}); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("walk %s: %w", t, err)
		}
	}

	// Check for orphaned source files (no wikilinks pointing to them)
	// A source page is orphaned if no other page links to it
	// This requires a two-pass approach: collect all links and all sources

	if issues == 0 {
		fmt.Println("✓ wiki lint: no issues found")
	} else {
		fmt.Printf("✗ wiki lint: %d issues found\n", issues)
	}
	return nil
}

func extractFrontmatterTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "title:") {
			return strings.TrimPrefix(strings.TrimSpace(line), "title:")
		}
	}
	return ""
}
