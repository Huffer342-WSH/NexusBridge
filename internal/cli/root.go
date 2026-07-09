package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/logging"
	"nexusbridge/internal/runtimeconfig"
	"nexusbridge/internal/server"
	webuiassets "nexusbridge/webui"
)

type options struct {
	configPath string
	staticDir  string
}

func NewRootCommand() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "nexusbridge",
		Short: "Fetch, cache, and expose NexusPHP torrent data",
	}
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "path to JSON config file")
	cmd.PersistentFlags().StringVar(&opts.staticDir, "static-dir", "", "override embedded WebUI with a static asset directory")

	cmd.AddCommand(newServeCommand(opts))
	cmd.AddCommand(newConfigCommand(opts))
	cmd.AddCommand(newFetchCommand(opts))
	cmd.AddCommand(newRunOnceCommand(opts))
	cmd.AddCommand(newRSSCommand())
	cmd.AddCommand(newQBCommand(opts))
	cmd.AddCommand(newOrganizeCommand(opts))
	return cmd
}

func newRunOnceCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "run-once <site>",
		Short: "Fetch, filter, and send matched new torrents to qBittorrent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cleanup, err := loadConfigAndSetupLogging(opts)
			if err != nil {
				return err
			}
			defer cleanup()
			app, err := core.NewApp(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer app.Close()
			result, err := app.RunOnce(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "run-once %s: %s fetched=%d changed=%d matched=%d sent=%d\n",
				result.SiteID, result.Status, result.Fetched, result.Changed, result.Matched, result.DownloadSent)
			return nil
		},
	}
}

func newServeCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the local HTTP API and WebUI server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cleanup, err := loadConfigAndSetupLogging(opts)
			if err != nil {
				return err
			}
			defer cleanup()
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			app, err := core.NewApp(ctx, cfg)
			if err != nil {
				return err
			}
			defer app.Close()
			app.StartAutomation(ctx)
			var srv *server.Server
			if opts.staticDir != "" {
				srv = server.New(cfg, app, opts.staticDir)
			} else {
				srv = server.NewWithAssets(cfg, app, webuiassets.Dist())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "NexusBridge listening on http://%s\n", cfg.Address())
			return srv.ListenAndServe(ctx)
		},
	}
}

func newConfigCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and validate configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Validate the JSON configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cleanup, err := loadConfigAndSetupLogging(opts)
			if err != nil {
				return err
			}
			defer cleanup()
			fmt.Fprintf(cmd.OutOrStdout(), "config ok: %s\n", cfg.Address())
			return nil
		},
	})
	return cmd
}

func newFetchCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "fetch <site>",
		Short: "Fetch torrents for a configured site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cleanup, err := loadConfigAndSetupLogging(opts)
			if err != nil {
				return err
			}
			defer cleanup()
			app, err := core.NewApp(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer app.Close()
			result, err := app.FetchSite(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "fetch %s: %s fetched=%d changed=%d\n", result.SiteID, result.Status, result.Fetched, result.Changed)
			return nil
		},
	}
}

func newRSSCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rss",
		Short: "Inspect RSS feed configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "rss is not implemented yet")
			return nil
		},
	}
}

func newQBCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "qb",
		Short: "Interact with qBittorrent WebUI",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "test",
		Short: "Test qBittorrent connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cleanup, err := loadConfigAndSetupLogging(opts)
			if err != nil {
				return err
			}
			defer cleanup()
			app, err := core.NewApp(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer app.Close()
			_, err = app.SyncQB(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "qBittorrent connection ok")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Sync qBittorrent completed torrents and create organize tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cleanup, err := loadConfigAndSetupLogging(opts)
			if err != nil {
				return err
			}
			defer cleanup()
			app, err := core.NewApp(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer app.Close()
			result, err := app.SyncQB(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "qb sync completed=%d organize_created=%d\n", result.Completed, result.OrganizeCreated)
			return nil
		},
	})
	return cmd
}

func newOrganizeCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organize",
		Short: "Organize completed downloads into media library",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "pending",
		Short: "Process pending organize tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cleanup, err := loadConfigAndSetupLogging(opts)
			if err != nil {
				return err
			}
			defer cleanup()
			app, err := core.NewApp(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer app.Close()
			result, err := app.OrganizePending(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "organize processed=%d failed=%d dry_run=%t\n", result.Processed, result.Failed, result.DryRun)
			return nil
		},
	})
	return cmd
}

// loadConfigAndSetupLogging 加载配置并初始化日志。
func loadConfigAndSetupLogging(opts *options) (config.Config, func() error, error) {
	result, err := runtimeconfig.Load(runtimeconfig.Options{ExplicitConfig: opts.configPath})
	if err != nil {
		return config.Config{}, nil, err
	}
	cfg := result.Config
	cleanup, err := logging.Setup(cfg.Logging)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, cleanup, nil
}
