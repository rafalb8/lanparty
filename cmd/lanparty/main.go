package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rafalb8/lanparty/internal/lanparty"
)

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, errHelp) {
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

func run(ctx context.Context, cfg Config, logger *slog.Logger) error {
	if cfg.ServerMode {
		hub, err := lanparty.NewHub(logger, cfg.CopyToken)
		if err != nil {
			return err
		}
		return hub.Run(ctx)
	}

	bridge, err := lanparty.NewBridge(ctx, cfg.Token, logger)
	if err != nil {
		return err
	}

	return bridge.Run(ctx)
}
