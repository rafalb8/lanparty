package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/songgao/water"
	"github.com/tailscale/tailcat"
)

type Bridge struct {
	cli    *tailcat.Client
	conn   net.Conn
	tap    *water.Interface
	logger *slog.Logger
}

func NewBridge(ctx context.Context, token string, ipCIDR string, logger *slog.Logger) (*Bridge, error) {
	fmt.Println("[*] Connecting to server...")

	cli := tailcat.NewClient(tailcat.Addr(token))
	cli.Logf = func(format string, args ...any) {
		logger.Debug(fmt.Sprintf("bridge: tailcat: "+format, args...))
	}

	conn, err := cli.DialUDPPort(ctx, MagicPort)
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("dial tailcat server: %w", err)
	}
	fmt.Println("[+] Connected to server!")

	tap, err := setupTap(ipCIDR)
	if err != nil {
		conn.Close()
		cli.Close()
		return nil, fmt.Errorf("bridge tap setup: %w", err)
	}
	fmt.Printf("[*] Created virtual LAN interface %s with IP %s\n", tap.Name(), ipCIDR)

	return &Bridge{
		cli:    cli,
		conn:   conn,
		tap:    tap,
		logger: logger,
	}, nil
}

func (b *Bridge) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		defer cancel()
		b.netToTapLoop()
	}()
	go func() {
		defer cancel()
		b.tapToNetLoop()
	}()

	<-ctx.Done()
	b.Close()
}

func (b *Bridge) Close() {
	b.tap.Close()
	b.conn.Close()

	if b.cli != nil {
		b.cli.Close()
	}
}

func (b *Bridge) netToTapLoop() {
	var buffer [MaxBufferSize]byte

	for {
		n, err := b.conn.Read(buffer[:])
		if err != nil {
			fmt.Println("[-] Disconnected from server.")
			return
		}

		_, err = b.tap.Write(buffer[:n])
		if err != nil {
			b.logger.Debug("bridge: failed to write frame to TAP", "err", err)
			return
		}
	}
}

func (b *Bridge) tapToNetLoop() {
	var buffer [MaxBufferSize]byte

	for {
		n, err := b.tap.Read(buffer[:])
		if err != nil {
			return
		}

		_, err = b.conn.Write(buffer[:n])
		if err != nil {
			b.logger.Debug("bridge: failed to write frame to network", "err", err)
			return
		}
	}
}
