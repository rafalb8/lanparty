package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"

	"github.com/songgao/water"
	"github.com/tailscale/tailcat"
)

type Bridge struct {
	cli     *tailcat.Client
	control net.Conn
	data    tailcat.ConnPacketConn
	tap     *water.Interface
	logger  *slog.Logger

	closeOnce sync.Once
}

func NewBridge(ctx context.Context, token string, logger *slog.Logger) (*Bridge, error) {
	tap, err := createTap()
	if err != nil {
		return nil, err
	}

	bridge := &Bridge{tap: tap, logger: logger}

	cli := tailcat.NewClient(tailcat.Addr(token))
	cli.Logf = func(format string, args ...any) {
		logger.Debug(fmt.Sprintf("bridge: tailcat: "+format, args...))
	}
	bridge.cli = cli

	control, err := cli.DialTCPPort(ctx, controlPort)
	if err != nil {
		bridge.Close()
		return nil, fmt.Errorf("dial control connection: %w", err)
	}
	bridge.control = control

	var host [1]byte
	_, err = io.ReadFull(control, host[:])
	if err != nil {
		bridge.Close()
		return nil, fmt.Errorf("read assigned address: %w", err)
	}

	if host[0] < 2 || host[0] > 254 {
		bridge.Close()
		return nil, fmt.Errorf("server assigned invalid address: 10.82.45.%d", host[0])
	}

	addr := netip.AddrFrom4([4]byte{10, 82, 45, host[0]})
	cidr := addr.String() + "/24"
	err = configureTap(tap, cidr)
	if err != nil {
		bridge.Close()
		return nil, fmt.Errorf("configure client TAP: %w", err)
	}

	data, err := cli.DialUDPPort(ctx, dataPort)
	if err != nil {
		bridge.Close()
		return nil, fmt.Errorf("dial data connection: %w", err)
	}
	bridge.data = data

	printClientInfo(tap.Name(), cidr)

	return bridge, nil
}

func (b *Bridge) Run(ctx context.Context) error {
	errCh := make(chan error, 3)

	go func() {
		errCh <- b.controlLoop()
	}()

	go func() {
		errCh <- b.netToTapLoop()
	}()

	go func() {
		errCh <- b.tapToNetLoop()
	}()

	select {
	case <-ctx.Done():
		b.Close()
		return nil
	case err := <-errCh:
		b.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	}
}

func (b *Bridge) controlLoop() error {
	_, err := io.Copy(io.Discard, b.control)
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
		return fmt.Errorf("control connection: %w", err)
	}

	return nil
}

func (b *Bridge) netToTapLoop() error {
	var buffer [maxBufferSize]byte

	for {
		n, err := b.data.Read(buffer[:])
		if err != nil {
			return err
		}

		if n == 0 {
			continue
		}

		written := 0
		for written < n {
			count, err := b.tap.Write(buffer[written:n])
			if err != nil {
				return fmt.Errorf("write frame to TAP: %w", err)
			}
			if count == 0 {
				return io.ErrShortWrite
			}
			written += count
		}
	}
}

func (b *Bridge) tapToNetLoop() error {
	var buffer [maxBufferSize]byte

	for {
		n, err := b.tap.Read(buffer[:])
		if err != nil {
			return err
		}

		if n == 0 {
			continue
		}

		count, err := b.data.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("write frame to network: %w", err)
		}
		if count != n {
			return io.ErrShortWrite
		}
	}
}

func (b *Bridge) Close() {
	b.closeOnce.Do(func() {
		if b.data != nil {
			b.data.Close()
		}

		if b.control != nil {
			b.control.Close()
		}

		if b.tap != nil {
			b.tap.Close()
		}

		if b.cli != nil {
			b.cli.Close()
		}
	})
}
