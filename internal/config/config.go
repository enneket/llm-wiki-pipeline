package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"

	"llm-wiki/pkg/database"
)

// Config holds all application config
type Config struct {
	Feeds   FeedsConfig  `json:"feeds"`
	Filter  FilterConfig `json:"filter"`
	Dedup   DedupConfig  `json:"dedup"`
	LLM     LLMConfig    `json:"llm"`
	Web     WebConfig    `json:"web"`
	Paths   PathsConfig  `json:"paths"`
	Process ProcessConfig `json:"process"`
}

type FeedsConfig struct {
	Feeds    []FeedEntry `yaml:"feeds" json:"feeds"`
	Interval string      `yaml:"interval" json:"interval"` // global interval for all feeds
}

type FeedEntry struct {
	Name string   `yaml:"name" json:"name"`
	URL  string   `yaml:"url" json:"url"`
	Tags []string `yaml:"tags" json:"tags"`
}

type FilterConfig struct {
	Mode        string            `yaml:"mode" json:"mode"`
	Keyword     KeywordFilter     `yaml:"keyword" json:"keyword"`
	LLMJudgment LLMJudgmentConfig `yaml:"llm_judgment" json:"llm_judgment"`
}

type KeywordFilter struct {
	MatchAny      bool     `yaml:"match_any" json:"match_any"`
	Tags          []string `yaml:"tags" json:"tags"`
	BlacklistTags []string `yaml:"blacklist_tags" json:"blacklist_tags"`
}

type LLMJudgmentConfig struct {
	Model         string  `yaml:"model" json:"model"`
	SampleRate    float64 `yaml:"sample_rate" json:"sample_rate"`
	MinConfidence float64 `yaml:"min_confidence" json:"min_confidence"`
}

type DedupConfig struct {
	URLExact         bool         `yaml:"url_exact" json:"url_exact"`
	ContentHash      bool         `yaml:"content_hash" json:"content_hash"`
	Vector           VectorConfig `yaml:"vector" json:"vector"`
	EmbeddingContext bool         `yaml:"embedding_context" json:"embedding_context"` // Use embedding search for wikilink context during ingest
}

type VectorConfig struct {
	Enabled      bool    `yaml:"enabled" json:"enabled"`
	Threshold    float64 `yaml:"threshold" json:"threshold"`
	Model        string  `yaml:"model" json:"model"`
	EmbeddingURL string  `yaml:"embedding_url" json:"embedding_url"`     // 可选，默认用 llm.base_url
	EmbeddingKey string  `yaml:"embedding_api_key" json:"embedding_api_key"` // 可选，默认用 llm.api_key
}

type LLMConfig struct {
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
	APIKey   string `yaml:"api_key" json:"api_key"`
	BaseURL  string `yaml:"base_url" json:"base_url"`
}

// WebConfig holds web server configuration
type WebConfig struct {
	Port     string `yaml:"port" json:"port"`
	APIToken string `yaml:"api_token" json:"api_token"` // Optional: Bearer token for API authentication
}

// PathsConfig holds data directory paths
type PathsConfig struct {
	Raw        string `yaml:"raw" json:"raw"`                 // Path to raw RSS data (default: data/raw)
	CleanedRaw string `yaml:"cleaned_raw" json:"cleaned_raw"` // Path to cleaned raw data (default: data/cleaned_raw)
	Wiki       string `yaml:"wiki" json:"wiki"`               // Path to wiki data (default: data/wiki)
	Reject     string `yaml:"reject" json:"reject"`           // Path to rejected data (default: data/reject)
}

// ProcessConfig holds LLM processing configuration
type ProcessConfig struct {
	Concurrency int `yaml:"concurrency" json:"concurrency"` // Number of concurrent LLM processing goroutines (default: 1)
}

var configFiles = []string{"feeds.yaml", "filter.yaml", "dedup.yaml", "llm.yaml"}

// Loader loads config and supports hot reload via fsnotify
type Loader struct {
	configDir string
	mu        sync.RWMutex
	cfg       *Config
	db        *database.DB
	mtimes    map[string]time.Time
}

// NewLoader creates a config loader
func NewLoader(configDir string) *Loader {
	// Auto-load .env from project root (cwd where CLI is run)
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "godotenv.Load (cwd): %v\n", err)
	}
	// Also try from config dir parent
	if absDir, err := filepath.Abs(configDir); err == nil {
		if err := godotenv.Load(filepath.Join(absDir, "..", ".env")); err != nil {
			// silently ignore - file may not exist
		}
	}
	return &Loader{configDir: configDir, mtimes: make(map[string]time.Time)}
}

// NewLoaderWithDB creates a config loader with database support
func NewLoaderWithDB(configDir string, db *database.DB) *Loader {
	return &Loader{
		configDir: configDir,
		db:        db,
		mtimes:    make(map[string]time.Time),
	}
}

// Load reads config from database or YAML files
func (l *Loader) Load() (*Config, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.db != nil {
		ctx := context.Background()
		cfg, err := LoadFromDB(ctx, l.db)
		if err != nil {
			return nil, err
		}
		l.cfg = cfg
		return cfg, nil
	}

	return l.loadFromYAML()
}

func (l *Loader) loadFromYAML() (*Config, error) {
	cfg := &Config{}
	for _, filename := range configFiles {
		path := filepath.Join(l.configDir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", filename, err)
		}
		data = expandEnv(data)
		// llm.yaml and dedup.yaml have a top-level key matching the section name
		// (e.g., "llm:" wraps llm config). We must unmarshal into the full *Config
		// so yaml can map the top-level key correctly via the struct tag.
		// Feeds and filter files have multiple top-level keys matching struct field tags.
		var target interface{}
		switch filename {
		case "llm.yaml", "dedup.yaml":
			target = cfg
		default:
			target = l.targetFor(filename, cfg)
		}
		if err := yaml.Unmarshal(data, target); err != nil {
			return nil, fmt.Errorf("parse %s: %w", filename, err)
		}
	}
	l.cfg = cfg
	return cfg, nil
}

// expandEnv expands ${VAR} and ${VAR:-default} patterns in YAML content
func expandEnv(data []byte) []byte {
	re := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(?::-([^}]*))?\}`)
	return re.ReplaceAllFunc(data, func(match []byte) []byte {
		m := re.FindStringSubmatch(string(match))
		key, defaultVal := m[1], m[2]
		val := os.Getenv(key)
		if val == "" {
			val = defaultVal
		}
		return []byte(val)
	})
}

func (l *Loader) targetFor(filename string, cfg *Config) interface{} {
	switch filename {
	case "feeds.yaml":
		return &cfg.Feeds // FeedsConfig.Feeds = []FeedEntry; yaml maps "feeds:" key into FeedsConfig
	case "filter.yaml":
		return &cfg.Filter
	case "dedup.yaml":
		return &cfg.Dedup
	case "llm.yaml":
		return &cfg.LLM
	}
	return nil
}

// Get returns the current config (read-locked)
func (l *Loader) Get() *Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cfg
}

// Reload re-reads all config files and returns the new config
func (l *Loader) Reload() (*Config, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.db != nil {
		ctx := context.Background()
		cfg, err := LoadFromDB(ctx, l.db)
		if err != nil {
			return nil, err
		}
		l.cfg = cfg
		return cfg, nil
	}

	return l.loadFromYAML()
}

// ReloadIfChanged checks mtimes and reloads only changed files
func (l *Loader) ReloadIfChanged() (*Config, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var changed bool
	for _, name := range configFiles {
		path := filepath.Join(l.configDir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		mtime := info.ModTime()
		if prev, ok := l.mtimes[name]; !ok || prev.Before(mtime) {
			l.mtimes[name] = mtime
			changed = true
		}
	}
	if !changed {
		return l.cfg, false, nil
	}
	newCfg, err := l.loadFromYAML()
	return newCfg, true, err
}

// LoadFromDB loads config from database
func LoadFromDB(ctx context.Context, db *database.DB) (*Config, error) {
	store := NewStore(db.Pool)
	cfg, err := store.LoadAll(ctx)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveToDB saves config to database
func SaveToDB(ctx context.Context, db *database.DB, cfg *Config) error {
	store := NewStore(db.Pool)

	if err := store.SaveCategory(ctx, "llm", cfg.LLM); err != nil {
		return err
	}
	if err := store.SaveCategory(ctx, "filter", cfg.Filter); err != nil {
		return err
	}
	if err := store.SaveCategory(ctx, "dedup", cfg.Dedup); err != nil {
		return err
	}
	if err := store.SaveCategory(ctx, "general", map[string]string{"interval": cfg.Feeds.Interval}); err != nil {
		return err
	}
	return nil
}

// LoadFromDBWithDefaults loads from DB, returns empty config if no settings exist
func LoadFromDBWithDefaults(ctx context.Context, db *database.DB) (*Config, error) {
	cfg, err := LoadFromDB(ctx, db)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &Config{}
	}
	// Apply defaults
	cfg.applyDefaults()
	return cfg, nil
}

// applyDefaults sets default values for empty fields
func (c *Config) applyDefaults() {
	if c.Paths.Raw == "" {
		c.Paths.Raw = "data/raw"
	}
	if c.Paths.CleanedRaw == "" {
		c.Paths.CleanedRaw = "data/cleaned_raw"
	}
	if c.Paths.Wiki == "" {
		c.Paths.Wiki = "data/wiki"
	}
	if c.Paths.Reject == "" {
		c.Paths.Reject = "data/reject"
	}
	if c.Web.Port == "" {
		c.Web.Port = "6006"
	}
	if c.Process.Concurrency <= 0 {
		c.Process.Concurrency = 1
	}
}
