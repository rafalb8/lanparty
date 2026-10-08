package main

import (
	"fmt"
	"net"

	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"tailscale.com/net/netmon"
)

const tapMTU = 1218

func createTap() (*water.Interface, error) {
	ifce, err := water.New(water.Config{DeviceType: water.TAP, Name: "lp%d"})
	if err != nil {
		return nil, fmt.Errorf("tap: create interface: %w", err)
	}

	name := ifce.Name()

	link, err := netlink.LinkByName(name)
	if err != nil {
		ifce.Close()
		return nil, fmt.Errorf("tap: find link %s: %w", name, err)
	}

	err = netlink.LinkSetMTU(link, tapMTU)
	if err != nil {
		ifce.Close()
		return nil, fmt.Errorf("tap: set mtu: %w", err)
	}

	netmon.RegisterInterfaceGetter(func() ([]netmon.Interface, error) {
		interfaces, err := net.Interfaces()
		if err != nil {
			return nil, err
		}

		result := make([]netmon.Interface, 0, len(interfaces))
		for i := range interfaces {
			if interfaces[i].Name == name {
				continue
			}
			result = append(result, netmon.Interface{Interface: &interfaces[i]})
		}

		return result, nil
	})

	return ifce, nil
}

func configureTap(ifce *water.Interface, cidr string) error {
	link, err := netlink.LinkByName(ifce.Name())
	if err != nil {
		return fmt.Errorf("tap: find link %s: %w", ifce.Name(), err)
	}

	addr, err := netlink.ParseAddr(cidr)
	if err != nil {
		return fmt.Errorf("tap: parse IP address %s: %w", cidr, err)
	}

	err = netlink.AddrAdd(link, addr)
	if err != nil {
		return fmt.Errorf("tap: add IP to link: %w", err)
	}

	err = netlink.LinkSetUp(link)
	if err != nil {
		return fmt.Errorf("tap: bring link up: %w", err)
	}

	return nil
}
