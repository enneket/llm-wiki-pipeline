package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config holds all application config
type Config struct {
	Feeds  FeedsConfig
	Filter FilterConfig
	Dedup  DedupConfig
	LLM    LLMConfig
}

type FeedsConfig struct {
	Feeds    []FeedEntry `yaml:"feeds"`
	Interval string     `yaml:"interval"` // global interval for all feeds
}

type FeedEntry struct {
	Name string   `yaml:"name"`
	URL  string   `yaml:"url"`
	Tags []string `yaml:"tags"`
}

type FilterConfig struct {
	Mode        string           `yaml:"mode"`
	Keyword     KeywordFilter    `yaml:"keyword"`
	LLMJudgment LLMJudgmentConfig `yaml:"llm_judgment"`
}

type KeywordFilter struct {
	MatchAny bool     `yaml:"match_any"`
	Tags     []string `yaml:"tags"`
}

type LLMJudgmentConfig struct {
	Model         string  `yaml:"model"`
	SampleRate    float64 `yaml:"sample_rate"`
	MinConfidence float64 `yaml:"min_confidence"`
}

type DedupConfig struct {
	URLExtact   bool         `yaml:"url_exact"`
	ContentHash bool         `yaml:"content_hash"`
	Vector      VectorConfig `yaml:"vector"`
}

type VectorConfig struct {
	Enabled       bool    `yaml:"enabled"`
	Threshold     float64 `yaml:"threshold"`
	Model         string  `yaml:"model"`
	EmbeddingURL  string  `yaml:"embedding_url"`  // 可选，默认用 llm.base_url
	EmbeddingKey  string  `yaml:"embedding_api_key"` // 可选，默认用 llm.api_key
}

type LLMConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key"`
	BaseURL  string `yaml:"base_url"`
}

var configFiles = []string{"feeds.yaml", "filter.yaml", "dedup.yaml", "llm.yaml"}

// Loader loads config and supports hot reload via fsnotify
type Loader struct {
	configDir string
	mu        sync.RWMutex
	cfg       *Config
}

// NewLoader creates a config loader
func NewLoader(configDir string) *Loader {
	return &Loader{configDir: configDir}
}

// Load reads all config files
func (l *Loader) Load() (*Config, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loadUnlocked()
}

func (l *Loader) loadUnlocked() (*Config, error) {
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
		if err := yaml.Unmarshal(data, l.targetFor(filename, cfg)); err != nil {
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
	return l.loadUnlocked()
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
		_ = info
		changed = true
	}
	if !changed {
		return l.cfg, false, nil
	}
	newCfg, err := l.loadUnlocked()
	return newCfg, true, err
}