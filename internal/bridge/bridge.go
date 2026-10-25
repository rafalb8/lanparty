package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"

	"github.com/rafalb8/lanparty/internal/config"
	"github.com/rafalb8/lanparty/internal/console"
	"github.com/rafalb8/lanparty/internal/privilege"
	"github.com/rafalb8/lanparty/internal/tap"
	"github.com/tailscale/tailcat"
)

type Config struct {
	Token    string
	Logger   *slog.Logger
	TAP      *tap.Interface
	Internet bool
	Subnet   netip.Prefix
}

type Bridge struct {
	tap     *tap.Interface
	cli     *tailcat.Client
	control net.Conn
	data    tailcat.ConnPacketConn

	closeOnce sync.Once
}

func New(ctx context.Context, cfg Config) (result *Bridge, err error) {
	iface := cfg.TAP
	bridge := &Bridge{tap: iface}
	defer func() {
		if err != nil {
			bridge.Close()
		}
	}()

	cli := tailcat.NewClient(tailcat.Addr(cfg.Token))
	logger := cfg.Logger
	cli.Logf = func(format string, args ...any) {
		logger.Debug(fmt.Sprintf("bridge: tailcat: "+format, args...))
	}
	bridge.cli = cli

	control, err := cli.DialTCPPort(ctx, config.ControlPort)
	if err != nil {
		return nil, fmt.Errorf("dial control connection: %w", err)
	}
	bridge.control = control
	stop := context.AfterFunc(ctx, func() { control.Close() })
	defer stop()

	var host [1]byte
	_, err = io.ReadFull(control, host[:])
	if err != nil {
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("read assigned address: %w", err)
	}

	if host[0] < 2 || host[0] > 254 {
		return nil, fmt.Errorf("server assigned invalid host number: %d", host[0])
	}

	addr := cfg.Subnet.Addr().As4()
	addr[3] = host[0]
	cidr := netip.PrefixFrom(netip.AddrFrom4(addr), cfg.Subnet.Bits()).String()
	err = iface.Configure(cidr)
	if err != nil {
		return nil, fmt.Errorf("configure client TAP: %w", err)
	}
	if !cfg.Internet {
		err = privilege.Keep(0)
		if err != nil {
			return nil, err
		}
	}

	data, err := cli.DialUDPPort(ctx, config.DataPort)
	if err != nil {
		return nil, fmt.Errorf("dial data connection: %w", err)
	}
	bridge.data = data

	console.PrintInfo(
		os.Stdout, "LAN Party",
		"Address:", cidr,
		"Interface:", iface.Name(),
		"Status:", "Connected",
	)

	return bridge, nil
}

func (b *Bridge) Run(ctx context.Context) error {
	defer b.Close()

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
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
			return err
		}

		return nil
	}
}

func (b *Bridge) controlLoop() error {
	_, err := io.Copy(io.Discard, b.control)
	if err != nil {
		return fmt.Errorf("control connection: %w", err)
	}

	return nil
}

func (b *Bridge) netToTapLoop() error {
	var buffer [tailcat.MaxUDPPayload]byte

	for {
		n, err := b.data.Read(buffer[:])
		if err != nil {
			return err
		}

		if n == 0 {
			continue
		}

		err = b.tap.WriteFrame(buffer[:n])
		if err != nil {
			return fmt.Errorf("write frame to TAP: %w", err)
		}
	}
}

func (b *Bridge) tapToNetLoop() error {
	var buffer [tailcat.MaxUDPPayload]byte

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

		if b.cli != nil {
			b.cli.Close()
		}
	})
}
