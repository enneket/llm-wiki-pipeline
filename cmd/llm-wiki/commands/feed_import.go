package commands

import (
	"fmt"
	"os"

	"llm-wiki/internal/config"
	"llm-wiki/pkg/feedutil"

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
	if feedutil.DetectFormat(data) == "opml" {
		parsed, err := feedutil.ParseOPML(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", filePath, err)
		}
		for _, f := range parsed {
			feeds = append(feeds, config.FeedEntry{
				Name: f.Name,
				URL:  f.URL,
				Tags: f.Tags,
			})
		}
	} else {
		parsed, err := feedutil.ParseURLList(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", filePath, err)
		}
		for _, f := range parsed {
			feeds = append(feeds, config.FeedEntry{
				Name: f.Name,
				URL:  f.URL,
				Tags: f.Tags,
			})
		}
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

// saveFeedsConfig writes the feeds config back to feeds.yaml
func saveFeedsConfig(cfg *config.Config) error {
	// Marshal with feeds: wrapper
	type feedsWrapper struct {
		Feeds    []config.FeedEntry `yaml:"feeds"`
		Interval string             `yaml:"interval"`
	}
	w := feedsWrapper{
		Feeds:    cfg.Feeds.Feeds,
		Interval: cfg.Feeds.Interval,
	}
	data, err := yaml.Marshal(w)
	if err != nil {
		return fmt.Errorf("marshal feeds: %w", err)
	}
	return os.WriteFile(configDir+"/feeds.yaml", data, 0644)
}
