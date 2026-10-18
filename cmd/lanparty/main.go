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
	if len(os.Args) > 1 && os.Args[1] == "exec" {
		err := execCommand(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

		return
	}

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
	var namespace *lanparty.Namespace
	var err error

	if cfg.Namespace {
		namespace, err = lanparty.NewNamespace()
		if err != nil {
			return err
		}
		defer namespace.Close()
	}

	var session interface {
		Run(context.Context) error
		Close()
	}

	if cfg.ServerMode {
		session, err = lanparty.NewHub(logger, cfg.CopyToken, namespace)
	} else {
		session, err = lanparty.NewBridge(ctx, cfg.Token, logger, namespace)
	}
	if err != nil {
		return err
	}
	defer session.Close()

	if namespace != nil {
		err = namespace.Publish()
		if err != nil {
			return err
		}
	}

	return session.Run(ctx)
}
