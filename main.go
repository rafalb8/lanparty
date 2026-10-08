package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/tailscale/tailcat"
	"golang.design/x/clipboard"
)

const (
	controlPort = 8245
	dataPort    = 8246

	serverCIDR = "10.82.45.1/24"

	maxBufferSize = tailcat.MaxUDPPayload
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
		hub, err := NewHub(logger, cfg.CopyToken)
		if err != nil {
			return err
		}
		return hub.Run(ctx)
	}

	bridge, err := NewBridge(ctx, cfg.Token, logger)
	if err != nil {
		return err
	}

	return bridge.Run(ctx)
}

func printInfo(wr io.Writer, title string, rows ...string) {
	if title != "" {
		fmt.Fprintln(wr)
		fmt.Fprintln(wr, title)
		fmt.Fprintln(wr, "──────────────")
	}

	w := tabwriter.NewWriter(wr, 0, 4, 1, ' ', 0)
	for i := 0; i < len(rows); i += 2 {
		fmt.Fprintf(w, "%s\t%s\n", rows[i], rows[i+1])
	}
	w.Flush()

	if title != "" {
		fmt.Fprintln(wr)
	}
}

func copyToClipboard(token string) error {
	err := clipboard.Init()
	if err != nil {
		return err
	}

	_, err = clipboard.Write(context.Background(), clipboard.FmtText, []byte(token))
	return err
}
