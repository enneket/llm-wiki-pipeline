package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"llm-wiki/internal/config"
	"llm-wiki/pkg/database"
	"llm-wiki/pkg/llm"
)

var queryRunCmd = &cobra.Command{
	Use:   "query <question>",
	Short: "Query the wiki with a question",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runQuery(strings.Join(args, " "))
	},
}

func runQuery(question string) error {
	cfg, err := config.NewLoader(configDir).Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	db, err := database.New(context.Background(), database.Config{DatabaseURL: dbURL})
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer db.Close()

	llmClient := llm.NewClient(cfg.LLM.APIKey, cfg.LLM.BaseURL, cfg.LLM.Model)

	ctx := context.Background()

	var results []llm.SearchResult

	embedURL := os.Getenv("EMBEDDING_BASE_URL")
	if embedURL != "" {
		// Vector search when embedding is configured
		embeddings, err := llmClient.EmbedSingle(ctx, question)
		if err != nil {
			return fmt.Errorf("embed question: %w", err)
		}
		results, err = llm.SearchEmbeddings(ctx, db.Pool, embeddings, 5)
		if err != nil {
			return fmt.Errorf("search wiki: %w", err)
		}
	} else {
		// Fallback to full-text search when embedding is not configured
		results, err = llm.SearchFullText(ctx, db.Pool, question, 5)
		if err != nil {
			return fmt.Errorf("search wiki (fulltext): %w", err)
		}
	}

	var contextBuilder strings.Builder
	if len(results) == 0 {
		contextBuilder.WriteString("(no related wiki pages found)")
	} else {
		for _, r := range results {
			contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", r.Title, truncate(r.Content, 300)))
		}
	}

	prompt := fmt.Sprintf(`You are answering a question about a personal wiki.

Question: %s

Related wiki pages:
%s

Based on the related wiki pages above, answer the question. If no relevant information is found, say so.`, question, contextBuilder.String())

	resp, err := llmClient.Complete(ctx, []llm.ChatMessage{
		{Role: "system", Content: "You answer questions based on a personal wiki."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return fmt.Errorf("llm answer: %w", err)
	}

	fmt.Println(resp)
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
