package commands

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"llm-wiki/internal/config"

	"gopkg.in/yaml.v3"
)

// addFeed adds a single feed to feeds.yaml
func addFeed(name, feedURL string, tags []string) error {
	loader := config.NewLoader(configDir)
	cfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Check for duplicate name
	for _, f := range cfg.Feeds.Feeds {
		if f.Name == name {
			return fmt.Errorf("feed %q already exists", name)
		}
	}

	cfg.Feeds.Feeds = append(cfg.Feeds.Feeds, config.FeedEntry{
		Name: name,
		URL:  feedURL,
		Tags: tags,
	})

	return saveFeedsConfig(cfg)
}

// importFeeds imports feeds from a file (OPML or plain URL list)
func importFeeds(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var feeds []config.FeedEntry

	// Detect format: OPML vs plain URL list
	if strings.Contains(string(data), "<opml") || strings.Contains(string(data), "<?xml") {
		feeds, err = parseOPML(data)
	} else {
		feeds, err = parseURLList(data)
	}
	if err != nil {
		return fmt.Errorf("parse %s: %w", filePath, err)
	}

	// Load existing config
	loader := config.NewLoader(configDir)
	cfg, err := loader.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	added := 0
	for _, f := range feeds {
		exists := false
		for _, existing := range cfg.Feeds.Feeds {
			if existing.URL == f.URL {
				exists = true
				break
			}
		}
		if !exists {
			cfg.Feeds.Feeds = append(cfg.Feeds.Feeds, f)
			added++
		}
	}

	if err := saveFeedsConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("✓ imported %d feeds (total %d)\n", added, len(cfg.Feeds.Feeds))
	return nil
}

// parseOPML parses OPML XML to extract RSS feed outlines
func parseOPML(data []byte) ([]config.FeedEntry, error) {
	var opml struct {
		XMLName xml.Name `xml:"opml"`
		Body    struct {
			Outlines []struct {
				XMLName xml.Name `xml:"outline"`
				Text    string  `xml:"text,attr"`
				Title   string  `xml:"title,attr"`
				XMLURL  string  `xml:"xmlUrl,attr"`
			} `xml:"body>outline"`
		} `xml:"body"`
	}

	if err := xml.Unmarshal(data, &opml); err != nil {
		return nil, fmt.Errorf("opml parse: %w", err)
	}

	var feeds []config.FeedEntry
	for _, o := range opml.Body.Outlines {
		if o.XMLURL == "" {
			continue
		}
		name := o.Title
		if name == "" {
			name = o.Text
		}
		if name == "" {
			u, _ := url.Parse(o.XMLURL)
			if u != nil {
				name = u.Host
			}
		}
		feeds = append(feeds, config.FeedEntry{
			Name: sanitizeName(name),
			URL:  o.XMLURL,
			Tags: []string{},
		})
	}
	return feeds, nil
}

// parseURLList parses a plain text file with one URL per line
func parseURLList(data []byte) ([]config.FeedEntry, error) {
	var feeds []config.FeedEntry
	urlRE := regexp.MustCompile(`https?://[^\s]+`)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matches := urlRE.FindAllString(line, -1)
		for _, match := range matches {
			u, err := url.Parse(match)
			if err != nil {
				continue
			}
			name := u.Host
			feeds = append(feeds, config.FeedEntry{
				Name: sanitizeName(name),
				URL:  match,
				Tags: []string{},
			})
		}
	}
	return feeds, nil
}

// sanitizeName converts a string to a valid feed name (lowercase, alphanumeric, underscores)
func sanitizeName(name string) string {
	result := strings.ToLower(name)
	result = regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(result, "_")
	result = strings.Trim(result, "_")
	if result == "" {
		result = "feed"
	}
	return result
}

// saveFeedsConfig writes the feeds config back to feeds.yaml
func saveFeedsConfig(cfg *config.Config) error {
	// Marshal with feeds: wrapper
	type feedsWrapper struct {
		Feeds   []config.FeedEntry `yaml:"feeds"`
		Interval string             `yaml:"interval"`
	}
	w := feedsWrapper{
		Feeds:   cfg.Feeds.Feeds,
		Interval: cfg.Feeds.Interval,
	}
	data, err := yaml.Marshal(w)
	if err != nil {
		return fmt.Errorf("marshal feeds: %w", err)
	}
	return os.WriteFile(configDir+"/feeds.yaml", data, 0644)
}