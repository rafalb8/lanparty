package tap

import (
	"fmt"
	"net"

	"github.com/rafalb8/lanparty/internal/namespace"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"tailscale.com/net/netmon"
)

const tapMTU = 1218

func Create() (*water.Interface, error) {
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

func Configure(ifce *water.Interface, cidr string, namespace *namespace.Namespace) error {
	target := netns.None()
	if namespace != nil {
		link, err := netlink.LinkByName(ifce.Name())
		if err != nil {
			return fmt.Errorf("tap: find link %s: %w", ifce.Name(), err)
		}

		err = netlink.LinkSetNsFd(link, int(namespace.Handle()))
		if err != nil {
			return fmt.Errorf("tap: move into namespace: %w", err)
		}

		target = namespace.Handle()
	}

	handle, err := netlink.NewHandleAt(target)
	if err != nil {
		return fmt.Errorf("tap: open netlink handle: %w", err)
	}
	defer handle.Close()

	link, err := handle.LinkByName(ifce.Name())
	if err != nil {
		return fmt.Errorf("tap: find link %s: %w", ifce.Name(), err)
	}

	addr, err := netlink.ParseAddr(cidr)
	if err != nil {
		return fmt.Errorf("tap: parse IP address %s: %w", cidr, err)
	}

	err = handle.AddrAdd(link, addr)
	if err != nil {
		return fmt.Errorf("tap: add IP to link: %w", err)
	}

	err = handle.LinkSetUp(link)
	if err != nil {
		return fmt.Errorf("tap: bring link up: %w", err)
	}

	if namespace != nil {
		loopback, err := handle.LinkByName("lo")
		if err != nil {
			return fmt.Errorf("tap: find loopback: %w", err)
		}

		err = handle.LinkSetUp(loopback)
		if err != nil {
			return fmt.Errorf("tap: bring loopback up: %w", err)
		}

		if namespace.HasInternet() {
			return nil
		}

		err = handle.RouteAdd(&netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
			Scope:     netlink.SCOPE_LINK,
		})
		if err != nil {
			return fmt.Errorf("tap: set default route: %w", err)
		}
	}

	return nil
}
