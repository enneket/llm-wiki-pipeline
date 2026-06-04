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
