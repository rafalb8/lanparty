package main

import (
	"fmt"

	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

// setupTap creates, addresses, and brings up a TAP interface.
func setupTap(ipCIDR string) (*water.Interface, error) {
	cfg := water.Config{DeviceType: water.TAP, Name: "lp%d"}

	ifce, err := water.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("tap: create interface: %w", err)
	}

	link, err := netlink.LinkByName(ifce.Name())
	if err != nil {
		ifce.Close()
		return nil, fmt.Errorf("tap: find link %s: %w", ifce.Name(), err)
	}

	addr, err := netlink.ParseAddr(ipCIDR)
	if err != nil {
		ifce.Close()
		return nil, fmt.Errorf("tap: parse IP address %s: %w", ipCIDR, err)
	}

	err = netlink.AddrAdd(link, addr)
	if err != nil {
		ifce.Close()
		return nil, fmt.Errorf("tap: add IP to link: %w", err)
	}

	err = netlink.LinkSetUp(link)
	if err != nil {
		ifce.Close()
		return nil, fmt.Errorf("tap: bring link up: %w", err)
	}

	err = netlink.LinkSetMTU(link, 1218)
	if err != nil {
		ifce.Close()
		return nil, fmt.Errorf("tap: setting mtu: %w", err)
	}

	return ifce, nil
}
