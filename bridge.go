package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/songgao/water"
)

// Point-to-Point Tunnel
type Bridge struct {
	conn net.Conn
	tap  *water.Interface
}

// Loop reads from server and writes to TAP
func (b *Bridge) Loop() {
	var length uint16
	var buffer [2 << 10]byte

	for {
		err := binary.Read(b.conn, binary.BigEndian, &length)
		if err != nil {
			fmt.Println("[-] Disconnected from server.")
			return
		}

		buf := buffer[:length]
		_, err = io.ReadFull(b.conn, buf)
		if err != nil {
			return
		}

		b.tap.Write(buf)
	}
}

// TapLoop reads from TAP and writes to server
func (b *Bridge) TapLoop() {
	var buffer [2 << 10]byte
	var lenBuf [2]byte

	for {
		n, err := b.tap.Read(buffer[:])
		if err != nil {
			return
		}
		binary.BigEndian.PutUint16(lenBuf[:], uint16(n))
		b.conn.Write(lenBuf[:])
		b.conn.Write(buffer[:n])
	}
}
