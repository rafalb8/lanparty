package namespace

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

type nsInternet struct {
	// shortcut: SIGKILL leaves host resources; add reconciliation if crash recovery is needed.
	link  netlink.Link
	table *nftables.Table
	dns   []*dnsRelay
}

func (n *nsInternet) setup(fd netns.NsHandle, subnet netip.Prefix) error {
	forwarding, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil {
		return err
	}
	if string(bytes.TrimSpace(forwarding)) != "1" {
		return fmt.Errorf("IPv4 forwarding is disabled; enable net.ipv4.ip_forward=1 and allow veth forwarding in your host firewall")
	}

	host := subnet.Addr().Next()
	guest := host.Next()

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: fmt.Sprintf("lpup%d", os.Getpid())},
		PeerName:  "lpwan", PeerNamespace: netlink.NsFd(fd),
	}
	err = netlink.LinkAdd(veth)
	if err != nil {
		return fmt.Errorf("create internet veth: %w", err)
	}
	n.link = veth
	err = netlink.AddrAdd(veth, internetAddress(host))
	if err != nil {
		return fmt.Errorf("address host veth: %w", err)
	}
	err = netlink.LinkSetUp(veth)
	if err != nil {
		return fmt.Errorf("bring host veth up: %w", err)
	}

	handle, err := netlink.NewHandleAt(fd)
	if err != nil {
		return err
	}
	defer handle.Close()
	peer, err := handle.LinkByName("lpwan")
	if err != nil {
		return err
	}
	err = handle.AddrAdd(peer, internetAddress(guest))
	if err != nil {
		return err
	}
	err = handle.LinkSetUp(peer)
	if err != nil {
		return err
	}
	loopback, err := handle.LinkByName("lo")
	if err != nil {
		return err
	}
	err = handle.LinkSetUp(loopback)
	if err != nil {
		return err
	}
	err = handle.RouteAdd(&netlink.Route{LinkIndex: peer.Attrs().Index, Gw: net.IP(host.AsSlice())})
	if err != nil {
		return fmt.Errorf("add internet default route: %w", err)
	}

	connection := &nftables.Conn{}
	table := connection.AddTable(&nftables.Table{Family: nftables.TableFamilyIPv4, Name: fmt.Sprintf("lanparty_%d", os.Getpid())})
	chain := connection.AddChain(&nftables.Chain{
		Name: "postrouting", Table: table, Type: nftables.ChainTypeNAT,
		Hooknum: nftables.ChainHookPostrouting, Priority: nftables.ChainPriorityNATSource,
	})
	connection.AddRule(&nftables.Rule{Table: table, Chain: chain, Exprs: internetNAT(veth.Name, guest)})
	err = connection.Flush()
	if err != nil {
		return fmt.Errorf("install internet NAT (requires kernel nftables support): %w", err)
	}
	n.table = table

	return n.setupDNS(fd)
}

func internetAddress(address netip.Addr) *netlink.Addr {
	return &netlink.Addr{IPNet: &net.IPNet{IP: net.IP(address.AsSlice()), Mask: net.CIDRMask(30, 32)}}
}

func internetNAT(name string, source netip.Addr) []expr.Any {
	interfaceName := make([]byte, 16)
	copy(interfaceName, name)
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: interfaceName},
		&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 12, Len: 4},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: source.AsSlice()},
		&expr.Masq{},
	}
}

func (n *nsInternet) close() {
	for _, relay := range n.dns {
		relay.close()
	}
	if n.table != nil {
		connection := &nftables.Conn{}
		connection.DelTable(n.table)
		err := connection.Flush()
		if err != nil {
			slog.Error("remove namespace NAT", "table", n.table.Name, "err", err)
		}
	}
	if n.link != nil {
		err := netlink.LinkDel(n.link)
		if err != nil {
			slog.Error("remove namespace veth", "interface", n.link.Attrs().Name, "err", err)
		}
	}
}
