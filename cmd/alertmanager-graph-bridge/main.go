// Command alertmanager-graph-bridge runs an HTTP server that forwards
// Prometheus Alertmanager webhook notifications as e-mails sent through the
// Microsoft Graph API.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/slauger/alertmanager-graph-bridge/internal/branding"
	"github.com/slauger/alertmanager-graph-bridge/internal/config"
	"github.com/slauger/alertmanager-graph-bridge/internal/graph"
	"github.com/slauger/alertmanager-graph-bridge/internal/mail"
	"github.com/slauger/alertmanager-graph-bridge/internal/server"
	"github.com/prometheus/client_golang/prometheus"
)

// version is the build version, overridden at link time with
// -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("alertmanager-graph-bridge", flag.ContinueOnError)
	cfgPath := fs.String("config", envOr("AGB_CONFIG", "config.yaml"),
		"path to the YAML configuration file")
	showVersion := fs.Bool("version", false, "print version information and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("%s %s (%s)\n", branding.ProductName, version, runtime.Version())
		return nil
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	logger := newLogger(cfg.Log)
	slog.SetDefault(logger)

	registry := prometheus.NewRegistry()
	registerBuildInfo(registry)

	graphClient, err := graph.New(graph.Options{
		TenantID:     cfg.Azure.TenantID,
		ClientID:     cfg.Azure.ClientID,
		ClientSecret: cfg.Azure.ClientSecret,
		TokenURL:     cfg.Azure.TokenURL,
		GraphBaseURL: cfg.Azure.GraphBaseURL,
		From:         cfg.Mail.From,
		Timeout:      cfg.Mail.SendTimeout.Std(),
	})
	if err != nil {
		return err
	}

	renderer, err := mail.NewRenderer(cfg.Mail.SubjectPrefix, cfg.Mail.Template)
	if err != nil {
		return err
	}

	srv := server.New(server.Options{
		Cfg:             cfg.Server,
		Sender:          graphClient,
		Renderer:        renderer,
		DefaultTo:       cfg.Mail.To,
		Logger:          logger,
		Registry:        registry,
		SendTimeout:     cfg.Mail.SendTimeout.Std(),
		SaveToSentItems: cfg.Mail.SaveToSentItems,
	})
	srv.SetReady(true)

	logger.Info("starting "+branding.ProductName,
		"port", cfg.Server.Port, "mailFrom", cfg.Mail.From, "template", cfg.Mail.Template)
	return srv.Run(ctx)
}

// registerBuildInfo exposes an agb_build_info gauge labelled with the build
// and Go versions.
func registerBuildInfo(reg prometheus.Registerer) {
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agb_build_info",
		Help: "Build information labelled by version and Go version.",
	}, []string{"version", "goversion"})
	buildInfo.WithLabelValues(version, runtime.Version()).Set(1)
	reg.MustRegister(buildInfo)
}

// envOr returns the environment variable value for key, or fallback if unset.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// newLogger builds an slog.Logger from the logging configuration.
func newLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
