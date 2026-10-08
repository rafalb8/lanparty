package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/netip"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/songgao/water"
	"github.com/tailscale/tailcat"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[netip.Addr]*client
	used    [256]bool
	closed  bool

	tap    *water.Interface
	srv    *tailcat.Server
	logger *slog.Logger

	closeOnce sync.Once
}

type client struct {
	peer    netip.Addr
	addr    netip.Addr
	control net.Conn

	dataMu sync.Mutex
	data   tailcat.ConnPacketConn
}

func NewHub(logger *slog.Logger, copyTokenToClipboard bool) (*Hub, error) {
	tap, err := createTap()
	if err != nil {
		return nil, err
	}

	hub := &Hub{
		clients: make(map[netip.Addr]*client),
		tap:     tap,
		logger:  logger,
	}

	hub.used[0] = true
	hub.used[1] = true
	hub.used[255] = true

	hub.srv = &tailcat.Server{
		Logf: func(format string, args ...any) {
			logger.Debug(fmt.Sprintf("tailcat: "+format, args...))
		},
		OnTCP: func(port uint16) func(net.Conn) {
			if port != controlPort {
				return nil
			}
			return hub.handleControl
		},
		OnUDP: func(port uint16) func(tailcat.ConnPacketConn) {
			if port != dataPort {
				return nil
			}
			return hub.handleData
		},
		UDPIdleTimeout: 24 * time.Hour,
	}

	err = configureTap(tap, serverCIDR)
	if err != nil {
		hub.Close()
		return nil, fmt.Errorf("configure server TAP: %w", err)
	}

	err = hub.srv.Start()
	if err != nil {
		hub.srv.Close()
		hub.tap.Close()
		return nil, fmt.Errorf("start tailcat server: %w", err)
	}

	token := string(hub.srv.TailcatAddr())
	printInfo(
		os.Stdout, "LAN Party Server",
		"Address:", serverCIDR,
		"Interface:", tap.Name(),
		"Control:", fmt.Sprintf("TCP :%d", controlPort),
		"Data:", fmt.Sprintf("UDP :%d", dataPort),
		"Token:", token,
	)
	if copyTokenToClipboard {
		err = copyToClipboard(token)
		if err != nil {
			logger.Warn("failed to copy server token to clipboard", "err", err)
		} else {
			fmt.Fprintln(os.Stdout, "Token copied to clipboard")
		}
	}

	return hub, nil
}

func (h *Hub) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		errCh <- h.tapLoop()
	}()

	select {
	case <-ctx.Done():
		h.Close()
		return nil

	case err := <-errCh:
		h.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	}
}

func (h *Hub) handleControl(conn net.Conn) {
	tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		h.logger.Debug("invalid control remote address", "address", conn.RemoteAddr())
		conn.Close()
		return
	}

	peer := tcpAddr.AddrPort().Addr()

	client, ok := h.addClient(peer, conn)
	if !ok {
		h.logger.Debug("rejecting client", "peer", peer)
		conn.Close()
		return
	}

	_, err := conn.Write([]byte{client.addr.As4()[3]})
	if err != nil {
		h.removeClient(peer, client)
		h.logger.Debug("failed to assign client address", "peer", peer, "err", err)
		return
	}

	cidr := client.addr.String() + "/24"
	printInfo(os.Stdout, "", "Player connected", cidr)
	h.logger.Debug("player connected", "peer", peer, "address", cidr)

	_, err = io.Copy(io.Discard, conn)
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
		h.logger.Debug("control connection ended", "peer", peer, "err", err)
	}

	h.removeClient(peer, client)
}

func (h *Hub) handleData(conn tailcat.ConnPacketConn) {
	udpAddr, ok := conn.RemoteAddr().(*net.UDPAddr)
	if !ok {
		h.logger.Debug("invalid data remote address", "address", conn.RemoteAddr())
		conn.Close()
		return
	}

	peer := udpAddr.AddrPort().Addr()
	h.mu.RLock()
	client := h.clients[peer]
	h.mu.RUnlock()
	if client == nil {
		h.logger.Debug("data connection without control connection", "peer", peer)
		conn.Close()
		return
	}

	client.setData(conn)
	defer client.clearData(conn)

	var buffer [maxBufferSize]byte
	for {
		n, err := conn.Read(buffer[:])
		if err != nil {
			if !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
				h.logger.Debug("data connection ended", "peer", peer, "err", err)
			}
			return
		}

		if n == 0 {
			continue
		}

		err = h.writeTap(buffer[:n])
		if err != nil {
			h.logger.Debug("failed to write frame to TAP", "peer", peer, "err", err)
			return
		}

		h.broadcast(buffer[:n], client)
	}
}

func (h *Hub) tapLoop() error {
	var buffer [maxBufferSize]byte

	for {
		n, err := h.tap.Read(buffer[:])
		if err != nil {
			return err
		}

		if n == 0 {
			continue
		}

		h.broadcast(buffer[:n], nil)
	}
}

func (h *Hub) addClient(peer netip.Addr, control net.Conn) (*client, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil, false
	}

	if _, exists := h.clients[peer]; exists {
		return nil, false
	}

	for host := byte(2); host <= 254; host++ {
		if h.used[host] {
			continue
		}

		h.used[host] = true
		client := &client{
			peer:    peer,
			addr:    netip.AddrFrom4([4]byte{10, 82, 45, host}),
			control: control,
		}
		h.clients[peer] = client
		return client, true
	}

	return nil, false
}

func (h *Hub) removeClient(peer netip.Addr, client *client) {
	h.mu.Lock()

	current, exists := h.clients[peer]
	if !exists || current != client {
		h.mu.Unlock()
		return
	}

	delete(h.clients, peer)
	h.used[client.addr.As4()[3]] = false
	h.mu.Unlock()

	client.close()

	cidr := client.addr.String() + "/24"
	printInfo(os.Stdout, "", "Player disconnected", cidr)
	h.logger.Debug("player disconnected", "peer", peer, "address", cidr)
}

func (h *Hub) broadcast(frame []byte, exclude *client) {
	h.mu.RLock()
	clients := slices.Collect(maps.Values(h.clients))
	h.mu.RUnlock()

	for _, client := range clients {
		if client == exclude {
			continue
		}

		err := client.writeData(frame)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			h.logger.Debug("failed to broadcast frame", "peer", client.peer, "err", err)
		}
	}
}

func (h *Hub) writeTap(frame []byte) error {
	written := 0
	for written < len(frame) {
		n, err := h.tap.Write(frame[written:])
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		written += n
	}
	return nil
}

func (h *Hub) Close() {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true

		clients := slices.Collect(maps.Values(h.clients))
		clear(h.clients)
		for _, client := range clients {
			h.used[client.addr.As4()[3]] = false
		}

		h.mu.Unlock()

		for _, client := range clients {
			client.close()
		}

		if h.srv != nil {
			h.srv.Close()
		}

		if h.tap != nil {
			h.tap.Close()
		}
	})
}

func (c *client) setData(data tailcat.ConnPacketConn) {
	c.dataMu.Lock()
	old := c.data
	c.data = data
	c.dataMu.Unlock()

	if old != nil {
		old.Close()
	}
}

func (c *client) clearData(data tailcat.ConnPacketConn) {
	c.dataMu.Lock()
	if c.data == data {
		c.data = nil
	}
	c.dataMu.Unlock()
}

func (c *client) writeData(frame []byte) error {
	c.dataMu.Lock()
	data := c.data
	c.dataMu.Unlock()

	if data == nil {
		return nil
	}

	n, err := data.Write(frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}

	return nil
}

func (c *client) close() {
	c.dataMu.Lock()
	data := c.data
	c.data = nil
	c.dataMu.Unlock()

	if data != nil {
		data.Close()
	}

	if c.control != nil {
		c.control.Close()
	}
}
