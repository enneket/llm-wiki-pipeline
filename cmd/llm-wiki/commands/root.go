package commands

import (
	"context"
	"fmt"
	"os/signal"
	"strings"
	"syscall"

	"llm-wiki/internal/config"
	"llm-wiki/internal/service"
	"llm-wiki/pkg/database"

	"github.com/spf13/cobra"
)

// rootCmd is set in init.go
var rootCmd = &cobra.Command{
	Use:   "llm-wiki",
	Short: "LLM Wiki Pipeline — RSS → Filter → Wiki",
	Long:  `自动化知识管理 Pipeline：RSS 订阅 → 筛选去重 → LLM Wiki 构建`,
}

var (
	configDir string
	dbURL    string
)

func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVarP(&configDir, "config", "c", "config/", "config directory")
	pf.StringVar(&dbURL, "db", "", "DATABASE_URL (env DATABASE_URL if empty)")

	rootCmd.AddCommand(feedCmd)
	rootCmd.AddCommand(filterCmd)
	rootCmd.AddCommand(wikiCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(queryRunCmd)
	rootCmd.AddCommand(reloadCmd)
}

// loadApp creates the service app with current config
func loadApp() (*service.App, error) {
	cfg, err := config.NewLoader(configDir).Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	dbCfg := database.Config{DatabaseURL: dbURL}
	db, err := database.New(context.Background(), dbCfg)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}

	return service.New(cfg, db), nil
}

// --- feed command ---

var feedCmd = &cobra.Command{Use: "feed", Short: "Manage RSS feeds"}

func init() {
	feedCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all configured feeds",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewLoader(configDir).Load()
			if err != nil {
				return err
			}
			for _, f := range cfg.Feeds.Feeds {
				fmt.Printf("%s\t%s\ttags=%v\n", f.Name, f.URL, f.Tags)
			}
			return nil
		},
	})
	feedCmd.AddCommand(&cobra.Command{
		Use:   "add <name> <url> [tags...]",
		Short: "Add a new RSS feed",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			url := args[1]
			tags := []string{}
			if len(args) > 2 {
				tags = strings.Split(args[2], ",")
			}
			return addFeed(name, url, tags)
		},
	})
	feedCmd.AddCommand(&cobra.Command{
		Use:   "import <file>",
		Short: "Batch import feeds from OPML or plain URL list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return importFeeds(args[0])
		},
	})
	feedCmd.AddCommand(&cobra.Command{
		Use:   "fetch",
		Short: "Manually trigger one fetch cycle",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadApp()
			if err != nil {
				return err
			}
			defer app.DB().Close()
			app.RunOnce()
			return nil
		},
	})
	feedCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show fetch status of all feeds",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("feed status — not yet implemented")
			return nil
		},
	})
}

// --- filter command ---

var filterCmd = &cobra.Command{Use: "filter", Short: "Run filter and dedup"}

func init() {
	filterCmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run filter on raw documents",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("filter run — not yet implemented (triggered automatically by scheduler)")
			return nil
		},
	})
	filterCmd.AddCommand(&cobra.Command{
		Use:   "tags",
		Short: "Show interest tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewLoader(configDir).Load()
			if err != nil {
				return err
			}
			fmt.Println("Primary:", cfg.Filter.Keyword.Tags)
			return nil
		},
	})
	filterCmd.AddCommand(&cobra.Command{
		Use:   "mode <keyword|llm_judgment>",
		Short: "Set or show filter mode",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cfg, err := config.NewLoader(configDir).Load()
				if err != nil {
					return err
				}
				fmt.Println("mode:", cfg.Filter.Mode)
				return nil
			}
			fmt.Println("mode change — not yet implemented")
			return nil
		},
	})
	filterCmd.AddCommand(&cobra.Command{
		Use:   "dedup",
		Short: "Show dedup statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("filter dedup — needs pg running")
			return nil
		},
	})
}

// --- wiki command ---

var wikiCmd = &cobra.Command{Use: "wiki", Short: "Manage LLM Wiki"}

func init() {
	wikiCmd.AddCommand(wikiLintCmd)
	wikiCmd.AddCommand(wikiIndexCmd)
	wikiCmd.AddCommand(&cobra.Command{
		Use:   "ingest",
		Short: "Ingest cleaned documents into wiki",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("wiki ingest — not yet implemented (triggered by filter)")
			return nil
		},
	})
	wikiCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show ingest queue status",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("wiki status — not yet implemented")
			return nil
		},
	})
}

// --- start command ---

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start background service (scheduler + pipeline)",
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := loadApp()
		if err != nil {
			return err
		}
		defer app.DB().Close()

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		fmt.Println("llm-wiki starting with", len(app.Scheduler().Feeds()), "feeds")
		return app.Start(ctx)
	},
}

// --- reload command ---

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Hot reload all config files",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := config.NewLoader(configDir)
		cfg, err := loader.Reload()
		if err != nil {
			return err
		}
		fmt.Printf("config reloaded — filter mode=%s feeds=%d\n", cfg.Filter.Mode, len(cfg.Feeds.Feeds))
		return nil
	},
}

var rootCmd2 = rootCmd // avoid init order issue