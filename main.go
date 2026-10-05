package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	MagicPort     = 8245
	IPRange       = "10.82.45.%d/24"
	MaxBufferSize = 65535 // max uint16
)

func main() {
	cfg := parseFlags()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logLevel := slog.LevelInfo
	if cfg.Verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	if cfg.ServerMode {
		ip := fmt.Sprintf(IPRange, 1)
		hub, err := NewHub(ip, logger)
		if err != nil {
			fmt.Printf("[!] Hub initialization failed: %v\n", err)
			return
		}
		hub.Run(ctx)
	} else {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		ip := fmt.Sprintf(IPRange, rng.Intn(253)+2)

		bridge, err := NewBridge(ctx, cfg.Token, ip, logger)
		if err != nil {
			fmt.Printf("[!] Bridge initialization failed: %v\n", err)
			return
		}
		bridge.Run(ctx)
	}

	fmt.Println("[-] Shutdown complete.")
}
