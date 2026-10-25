package tap

import (
	"fmt"
	"io"
	"net"

	"github.com/rafalb8/lanparty/internal/namespace"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"tailscale.com/net/netmon"
)

const tapMTU = 1218

type Interface struct {
	*water.Interface
	handle    *netlink.Handle
	namespace *namespace.Namespace
}

// New completes namespace transitions while CAP_SYS_ADMIN is still available.
func New(ns *namespace.Namespace) (*Interface, error) {
	iface, err := create()
	if err != nil {
		return nil, err
	}
	p := &Interface{Interface: iface, namespace: ns}
	if ns != nil {
		var link netlink.Link
		link, err = netlink.LinkByName(iface.Name())
		if err == nil {
			err = netlink.LinkSetNsFd(link, int(ns.Handle()))
		}
		if err == nil {
			p.handle, err = ns.OpenRouteHandle()
		}
	} else {
		p.handle, err = netlink.NewHandle(unix.NETLINK_ROUTE)
	}
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("tap: prepare configuration: %w", err)
	}
	return p, nil
}

func (p *Interface) Close() {
	if p.handle != nil {
		p.handle.Close()
		p.handle = nil
	}
	p.Interface.Close()
}

func create() (*water.Interface, error) {
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

func (p *Interface) Configure(cidr string) error {
	ifce, handle, namespace := p.Interface, p.handle, p.namespace
	defer func() {
		handle.Close()
		p.handle = nil
	}()

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

	if namespace == nil {
		return nil
	}

	loopback, err := handle.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("tap: find loopback: %w", err)
	}

	err = handle.LinkSetUp(loopback)
	if err != nil {
		return fmt.Errorf("tap: bring loopback up: %w", err)
	}

	if namespace.InternetEnabled() {
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

	return nil
}

func (p *Interface) WriteFrame(frame []byte) error {
	for len(frame) > 0 {
		written, err := p.Write(frame)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}

		frame = frame[written:]
	}

	return nil
}
