package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rafalb8/lanparty/internal/bridge"
	"github.com/rafalb8/lanparty/internal/config"
	"github.com/rafalb8/lanparty/internal/hub"
	"github.com/rafalb8/lanparty/internal/launcher"
	"github.com/rafalb8/lanparty/internal/namespace"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "exec" {
		err := launcher.Exec(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, config.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	level := slog.LevelInfo
	if cfg.Verbose {
		level = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	err = run(ctx, cfg, logger)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("lanparty stopped", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	var networkNamespace *namespace.Namespace
	var err error

	if cfg.Namespace {
		networkNamespace, err = namespace.New(cfg.Internet, cfg.InternetSubnet)
		if err != nil {
			return err
		}
		defer networkNamespace.Close()
	}

	var session interface {
		Run(context.Context) error
		Close()
	}

	if cfg.ServerMode {
		session, err = hub.New(logger, networkNamespace, cfg.Subnet)
	} else {
		session, err = bridge.New(ctx, bridge.Config{
			Token:     cfg.Token,
			Logger:    logger,
			Namespace: networkNamespace,
			Subnet:    cfg.Subnet,
		})
	}
	if err != nil {
		return err
	}
	defer session.Close()

	if networkNamespace != nil {
		err = networkNamespace.Publish()
		if err != nil {
			return err
		}
	}

	return session.Run(ctx)
}
