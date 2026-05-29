package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles DB-backed settings
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new settings store
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// LoadAll loads all settings from DB and returns a Config
func (s *Store) LoadAll(ctx context.Context) (*Config, error) {
	rows, err := s.pool.Query(ctx, "SELECT category, data FROM settings")
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	defer rows.Close()

	cfg := &Config{}
	for rows.Next() {
		var category string
		var data []byte
		if err := rows.Scan(&category, &data); err != nil {
			return nil, fmt.Errorf("scan settings: %w", err)
		}
		if err := applyCategory(cfg, category, data); err != nil {
			return nil, fmt.Errorf("apply %s: %w", category, err)
		}
	}
	return cfg, nil
}

// SaveCategory saves a single category to DB
func (s *Store) SaveCategory(ctx context.Context, category string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO settings (category, data, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (category) DO UPDATE SET data = $2, updated_at = NOW()
	`, category, jsonData)
	if err != nil {
		return fmt.Errorf("upsert settings: %w", err)
	}
	return nil
}

// GetCategory returns raw JSON for a category
func (s *Store) GetCategory(ctx context.Context, category string) (json.RawMessage, error) {
	var data json.RawMessage
	err := s.pool.QueryRow(ctx, "SELECT data FROM settings WHERE category = $1", category).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("get category: %w", err)
	}
	return data, nil
}

// GetAll returns all settings as a map
func (s *Store) GetAll(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.pool.Query(ctx, "SELECT category, data FROM settings")
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	defer rows.Close()

	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var category string
		var data json.RawMessage
		if err := rows.Scan(&category, &data); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result[category] = data
	}
	return result, nil
}

func applyCategory(cfg *Config, category string, data []byte) error {
	switch category {
	case "llm":
		return json.Unmarshal(data, &cfg.LLM)
	case "filter":
		return json.Unmarshal(data, &cfg.Filter)
	case "dedup":
		return json.Unmarshal(data, &cfg.Dedup)
	case "general":
		var g struct {
			Interval string `json:"interval"`
		}
		if err := json.Unmarshal(data, &g); err != nil {
			return err
		}
		cfg.Feeds.Interval = g.Interval
		return nil
	default:
		return fmt.Errorf("unknown category: %s", category)
	}
}
