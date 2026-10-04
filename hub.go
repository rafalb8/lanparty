package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

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
		OnTCP: func(port uint16) func(net.Conn) {
			if port != MagicPort {
				return nil
			}
			return hub.add
		},
		Logf: func(format string, args ...any) {
			logger.Debug(fmt.Sprintf("tailcat: "+format, args...))
		},
	}

	if err := hub.srv.Start(); err != nil {
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

	var lenBuf [2]byte
	var buffer [MaxBufferSize]byte

	for {
		if _, err := io.ReadFull(c, lenBuf[:]); err != nil {
			return
		}

		length := binary.BigEndian.Uint16(lenBuf[:])
		if length > MaxBufferSize {
			h.logger.Debug("hub: dropping oversized frame from client", "bytes", length)
			return
		}

		buf := buffer[:length]
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}

		if _, err := h.tap.Write(buf); err != nil {
			h.logger.Debug("hub: failed to write frame to TAP", "err", err)
			return
		}

		h.broadcast(buf, c)
	}
}

func (h *Hub) broadcast(frame []byte, exclude net.Conn) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	payload := make([]byte, 2+len(frame))
	binary.BigEndian.PutUint16(payload[0:2], uint16(len(frame)))
	copy(payload[2:], frame)

	for c := range h.clients {
		if c != exclude {
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := c.Write(payload); err != nil {
				h.logger.Debug("hub: failed to broadcast to client", "err", err)
			}
			c.SetWriteDeadline(time.Time{})
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
