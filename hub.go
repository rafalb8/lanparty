package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/songgao/water"
	"github.com/tailscale/tailcat"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[net.Conn]struct{}
	tap     *water.Interface
	srv     *tailcat.Server
	logger  *slog.Logger
}

func NewHub(ipCIDR string, logger *slog.Logger) (*Hub, error) {
	tap, err := setupTap(ipCIDR)
	if err != nil {
		return nil, fmt.Errorf("tap setup: %w", err)
	}
	fmt.Printf("[*] Created virtual LAN interface %s with IP %s\n", tap.Name(), ipCIDR)

	hub := &Hub{
		clients: make(map[net.Conn]struct{}),
		tap:     tap,
		logger:  logger,
	}

	hub.srv = &tailcat.Server{
		Logf: func(format string, args ...any) {
			logger.Debug(fmt.Sprintf("tailcat: "+format, args...))
		},
	}

	hub.srv.OnUDP = func(port uint16) func(tailcat.ConnPacketConn) {
		if port != MagicPort {
			return nil
		}
		return func(c tailcat.ConnPacketConn) { hub.add(c) }
	}

	err = hub.srv.Start()
	if err != nil {
		tap.Close()
		return nil, fmt.Errorf("start tailcat server: %w", err)
	}

	fmt.Println("[+] Server token:", hub.srv.TailcatAddr())
	return hub, nil
}

func (h *Hub) add(c net.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	fmt.Println("[+] Player connected!")

	go h.handleClient(c)
}

func (h *Hub) remove(c net.Conn) {
	c.Close()
	h.mu.Lock()
	if _, exists := h.clients[c]; exists {
		delete(h.clients, c)
		fmt.Println("[-] Player disconnected.")
	}
	h.mu.Unlock()
}

func (h *Hub) handleClient(c net.Conn) {
	defer h.remove(c)
	var buffer [MaxBufferSize]byte

	for {
		n, err := c.Read(buffer[:])
		if err != nil {
			return
		}

		_, err = h.tap.Write(buffer[:n])
		if err != nil {
			h.logger.Debug("hub: failed to write frame to TAP", "err", err)
			return
		}

		h.broadcast(buffer[:n], c)
	}
}

func (h *Hub) broadcast(frame []byte, exclude net.Conn) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if c != exclude {
			_, err := c.Write(frame)
			if err != nil {
				h.logger.Debug("hub: failed to broadcast to client", "err", err)
			}
		}
	}
}

func (h *Hub) tapLoop() {
	var buffer [MaxBufferSize]byte
	for {
		n, err := h.tap.Read(buffer[:])
		if err != nil {
			return
		}
		h.broadcast(buffer[:n], nil)
	}
}

func (h *Hub) Run(ctx context.Context) {
	go h.tapLoop()

	<-ctx.Done()
	h.Close()
}

func (h *Hub) Close() {
	h.mu.Lock()
	for c := range h.clients {
		c.Close()
		delete(h.clients, c)
	}
	h.mu.Unlock()

	if h.srv != nil {
		h.srv.Close()
	}

	h.tap.Close()
}
