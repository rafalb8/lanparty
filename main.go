package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/songgao/water"
	"github.com/tailscale/tailcat"
	"github.com/vishvananda/netlink"
)

const (
	MagicPort = 8245
	IPRange   = "10.82.45.%d/24"
)

func setupTap(ipCIDR string) *water.Interface {
	ifce, err := water.New(water.Config{DeviceType: water.TAP})
	if err != nil {
		log.Fatalf("[!] Failed to create TAP: %v", err)
	}

	link, err := netlink.LinkByName(ifce.Name())
	if err != nil {
		log.Fatalf("[!] Failed to find link %s: %v", ifce.Name(), err)
	}

	addr, err := netlink.ParseAddr(ipCIDR)
	if err != nil {
		log.Fatalf("[!] Failed to parse address: %v", err)
	}

	err = netlink.AddrAdd(link, addr)
	if err != nil {
		log.Fatalf("[!] Failed to set IP: %v", err)
	}

	err = netlink.LinkSetUp(link)
	if err != nil {
		log.Fatalf("[!] Failed to bring link up: %v", err)
	}

	fmt.Printf("[*] Created virtual LAN interface %s with IP %s\n", ifce.Name(), ipCIDR)
	return ifce
}

func server(tap *water.Interface) {
	hub := &Hub{tap: tap}
	go hub.TapLoop()

	srv := &tailcat.Server{
		OnTCP: func(port uint16) func(net.Conn) {
			if port != MagicPort {
				return nil
			}
			return hub.Add
		},
		// Logf: func(format string, args ...any) {},
	}

	err := srv.Start()
	if err != nil {
		log.Fatalf("[!] Failed to start server: %v", err)
	}

	fmt.Println("[+] Token:", srv.TailcatAddr())
}

func client(token tailcat.Addr, tap *water.Interface) {
	fmt.Println("[*] Connecting to server...")

	cli := tailcat.NewClient(token)
	c, err := cli.DialTCPPort(context.Background(), MagicPort)
	if err != nil {
		log.Fatalf("[!] Failed to connect: %v", err)
	}
	fmt.Println("[+] Connected!")

	bridge := &Bridge{tap: tap, conn: c}
	go bridge.TapLoop()
	go bridge.Loop()
}

func main() {
	if len(os.Args) > 2 {
		fmt.Printf("Usage: %s [token]\n", os.Args[0])
		os.Exit(1)
	}

	id := 1
	serverMode := len(os.Args) == 1
	if !serverMode {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		id = rng.Intn(253) + 2
	}

	ip := fmt.Sprintf(IPRange, id)
	tap := setupTap(ip)
	defer tap.Close()

	if serverMode {
		server(tap)
	} else {
		token := tailcat.Addr(os.Args[1])
		client(token, tap)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("[*] Shutting down...")
}
