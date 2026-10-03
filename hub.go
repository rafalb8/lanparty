package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/songgao/water"
)

// Multi-port Virtual Switch
type Hub struct {
	mu      sync.Mutex
	clients []net.Conn
	tap     *water.Interface
}

func (h *Hub) Remove(c net.Conn) {
	c.Close()
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, client := range h.clients {
		if client == c {
			h.clients = append(h.clients[:i], h.clients[i+1:]...)
			break
		}
	}
	fmt.Println("[-] Player disconnected.")
}

func (h *Hub) Add(c net.Conn) {
	h.mu.Lock()
	h.clients = append(h.clients, c)
	h.mu.Unlock()

	fmt.Println("[+] Player connected!")
	go h.loop(c)
}

// loop reads from client and writes to TAP
func (h *Hub) loop(c net.Conn) {
	defer h.Remove(c)

	var length uint16
	var buffer [2 << 10]byte

	for {
		err := binary.Read(c, binary.BigEndian, &length)
		if err != nil {
			return
		}

		buf := buffer[:length]
		_, err = io.ReadFull(c, buf)
		if err != nil {
			return
		}

		// Inject frame into local OS
		h.tap.Write(buf)
		h.broadcast(buf, c)
	}
}

// broadcast sends frame to other connected clients
func (h *Hub) broadcast(frame []byte, exclude net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(frame)))

	for _, c := range h.clients {
		if c != exclude {
			c.Write(lenBuf[:])
			c.Write(frame)
		}
	}
}

// TapLoop reads from TAP and broadcasts to clients
func (h *Hub) TapLoop() {
	var buffer [2 << 10]byte
	for {
		n, err := h.tap.Read(buffer[:])
		if err != nil {
			return
		}
		h.broadcast(buffer[:n], nil)
	}
}
