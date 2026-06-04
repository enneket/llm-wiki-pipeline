package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"llm-wiki/internal/config"
	"llm-wiki/pkg/llm"
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

func (s *Server) handleTestLLM(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get current LLM config
	store := config.NewStore(s.db.Pool)
	data, err := store.GetCategory(ctx, "llm")
	if err != nil {
		http.Error(w, "failed to load LLM config", http.StatusInternalServerError)
		return
	}

	var llmCfg struct {
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
	}
	if err := json.Unmarshal(data, &llmCfg); err != nil {
		http.Error(w, "invalid LLM config", http.StatusBadRequest)
		return
	}

	if llmCfg.APIKey == "" || llmCfg.BaseURL == "" || llmCfg.Model == "" {
		http.Error(w, "LLM config incomplete", http.StatusBadRequest)
		return
	}

	// Create client and test
	client := llm.NewClient(llmCfg.APIKey, llmCfg.BaseURL, llmCfg.Model)
	resp, err := client.Complete(ctx, []llm.ChatMessage{
		{Role: "user", Content: "Say 'Hello' in one word."},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("LLM test failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "response": resp})
}

func (s *Server) handleTestEmbedding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get current config
	store := config.NewStore(s.db.Pool)
	llmData, err := store.GetCategory(ctx, "llm")
	if err != nil {
		http.Error(w, "failed to load LLM config", http.StatusInternalServerError)
		return
	}
	dedupData, err := store.GetCategory(ctx, "dedup")
	if err != nil {
		http.Error(w, "failed to load dedup config", http.StatusInternalServerError)
		return
	}

	var llmCfg struct {
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
	}
	if err := json.Unmarshal(llmData, &llmCfg); err != nil {
		http.Error(w, "invalid LLM config", http.StatusBadRequest)
		return
	}

	var dedupCfg struct {
		Vector struct {
			Model        string `json:"model"`
			EmbeddingURL string `json:"embedding_url"`
			EmbeddingKey string `json:"embedding_api_key"`
		} `json:"vector"`
	}
	if err := json.Unmarshal(dedupData, &dedupCfg); err != nil {
		http.Error(w, "invalid dedup config", http.StatusBadRequest)
		return
	}

	// Use embedding config or fallback to LLM config
	embedURL := dedupCfg.Vector.EmbeddingURL
	embedKey := dedupCfg.Vector.EmbeddingKey
	embedModel := dedupCfg.Vector.Model
	if embedURL == "" {
		embedURL = llmCfg.BaseURL
	}
	if embedKey == "" {
		embedKey = llmCfg.APIKey
	}
	if embedModel == "" {
		embedModel = llmCfg.Model
	}

	if embedKey == "" || embedURL == "" {
		http.Error(w, "embedding config incomplete", http.StatusBadRequest)
		return
	}

	// Create client and test
	client := llm.NewClientWithEmbed(llmCfg.APIKey, llmCfg.BaseURL, llmCfg.Model, embedURL, embedKey)
	embeddings, err := client.Embed(ctx, []string{"Hello world"})
	if err != nil {
		http.Error(w, fmt.Sprintf("Embedding test failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"dimension": len(embeddings[0]),
	})
}
