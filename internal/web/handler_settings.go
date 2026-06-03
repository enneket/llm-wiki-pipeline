package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"llm-wiki/internal/config"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := config.NewStore(s.db.Pool)

	all, err := store.GetAll(ctx)
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(all)
}

func (s *Server) handleGetSettingCategory(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	ctx := r.Context()
	store := config.NewStore(s.db.Pool)

	data, err := store.GetCategory(ctx, category)
	if err != nil {
		http.Error(w, "category not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handleUpdateSettingCategory(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	ctx := r.Context()

	// Read body
	var data json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate
	if err := validateSettings(category, data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Save to DB
	store := config.NewStore(s.db.Pool)
	if err := store.SaveCategory(ctx, category, data); err != nil {
		log.Printf("[settings] save %s: %v", category, err)
		http.Error(w, "failed to save", http.StatusInternalServerError)
		return
	}

	// Reload config in memory (will be implemented in Task 5)
	if err := s.reloadConfig(ctx); err != nil {
		log.Printf("[settings] reload config: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) reloadConfig(ctx context.Context) error {
	cfg, err := config.LoadFromDB(ctx, s.db)
	if err != nil {
		return fmt.Errorf("reload config from db: %w", err)
	}
	s.cfg = cfg

	// Update LLM client if config changed
	if s.onLLMUpdate != nil {
		s.onLLMUpdate(cfg.LLM.APIKey, cfg.LLM.BaseURL, cfg.LLM.Model)
	}

	return nil
}

func validateSettings(category string, data json.RawMessage) error {
	switch category {
	case "llm":
		var llm struct {
			Model   string `json:"model"`
			BaseURL string `json:"base_url"`
		}
		if err := json.Unmarshal(data, &llm); err != nil {
			return err
		}
		if llm.Model == "" {
			return fmt.Errorf("model is required")
		}
		if llm.BaseURL == "" {
			return fmt.Errorf("base_url is required")
		}
	case "filter":
		var filter struct {
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal(data, &filter); err != nil {
			return err
		}
		if filter.Mode != "keyword" && filter.Mode != "llm_judgment" {
			return fmt.Errorf("mode must be 'keyword' or 'llm_judgment'")
		}
	case "dedup":
		var dedup struct {
			Vector struct {
				Threshold float64 `json:"threshold"`
			} `json:"vector"`
		}
		if err := json.Unmarshal(data, &dedup); err != nil {
			return err
		}
		if dedup.Vector.Threshold < 0 || dedup.Vector.Threshold > 1 {
			return fmt.Errorf("threshold must be between 0 and 1")
		}
	case "general":
		var g struct {
			Interval string `json:"interval"`
		}
		if err := json.Unmarshal(data, &g); err != nil {
			return err
		}
		if g.Interval == "" {
			return fmt.Errorf("interval is required")
		}
	default:
		return fmt.Errorf("unknown category: %s", category)
	}
	return nil
}
