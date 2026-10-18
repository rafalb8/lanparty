package namespace

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/vishvananda/netns"
	"tailscale.com/net/dns/resolvconffile"
)

const dnsTimeout = 5 * time.Second

type dnsRelay struct {
	udp      net.PacketConn
	tcp      net.Listener
	upstream string
	context  context.Context
	cancel   context.CancelFunc
	workers  sync.WaitGroup
	slots    chan struct{}
}

func (n *nsInternet) setupDNS(fd netns.NsHandle) error {
	config, err := resolvconffile.ParseFile("/etc/resolv.conf")
	if err != nil {
		return fmt.Errorf("read host DNS: %w", err)
	}

	nameservers := config.Nameservers
	if len(nameservers) == 0 {
		nameservers = []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	}

	usable := false
	var loopbacks []netip.Addr
	for _, address := range nameservers {
		usable = usable || address.Is4() || address.IsLoopback()
		if address.IsLoopback() && !slices.Contains(loopbacks, address) {
			loopbacks = append(loopbacks, address)
		}
	}

	if !usable {
		return fmt.Errorf("host DNS has only remote IPv6 nameservers; configure an IPv4 or loopback nameserver for --internet")
	}
	if len(loopbacks) == 0 {
		return nil
	}

	err = inNamespace(fd, func() error {
		// Allow DNS port binding with CAP_NET_ADMIN, only in this namespace.
		err := os.WriteFile("/proc/sys/net/ipv4/ip_unprivileged_port_start", []byte("0"), 0)
		if err != nil {
			return fmt.Errorf("allow namespace DNS sockets: %w", err)
		}

		for _, address := range loopbacks {
			endpoint := net.JoinHostPort(address.String(), "53")
			udp, err := net.ListenPacket("udp", endpoint)
			if err != nil {
				return err
			}

			tcp, err := net.Listen("tcp", endpoint)
			if err != nil {
				udp.Close()
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			n.dns = append(n.dns, &dnsRelay{
				udp: udp, tcp: tcp, upstream: endpoint,
				context: ctx, cancel: cancel, slots: make(chan struct{}, 64),
			})
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("configure namespace DNS: %w", err)
	}

	// Start forwarding only after the setup thread has returned to the host.
	for _, relay := range n.dns {
		relay.workers.Add(2)
		go relay.serveUDP()
		go relay.serveTCP()
	}

	return nil
}

func (r *dnsRelay) close() {
	r.cancel()
	r.udp.Close()
	r.tcp.Close()
	r.workers.Wait()
}

func (r *dnsRelay) serveUDP() {
	defer r.workers.Done()

	buffer := make([]byte, 65535)
	for {
		size, peer, err := r.udp.ReadFrom(buffer)
		if err != nil {
			return
		}

		select {
		case r.slots <- struct{}{}:
		default:
			continue
		}

		packet := buffer
		buffer = make([]byte, 65535)
		r.workers.Add(1)
		go r.forwardUDP(packet, size, peer)
	}
}

func (r *dnsRelay) forwardUDP(buffer []byte, size int, peer net.Addr) {
	defer r.workers.Done()
	defer func() { <-r.slots }()

	connection, err := r.dial("udp")
	if err != nil {
		return
	}
	defer connection.Close()
	stop := context.AfterFunc(r.context, func() { connection.Close() })
	defer stop()

	_, err = connection.Write(buffer[:size])
	if err != nil {
		return
	}

	count, err := connection.Read(buffer)
	if err == nil {
		r.udp.WriteTo(buffer[:count], peer)
	}
}

func (r *dnsRelay) serveTCP() {
	defer r.workers.Done()

	for {
		client, err := r.tcp.Accept()
		if err != nil {
			return
		}

		select {
		case r.slots <- struct{}{}:
		default:
			client.Close()
			continue
		}

		r.workers.Add(1)
		go r.forwardTCP(client)
	}
}

func (r *dnsRelay) forwardTCP(client net.Conn) {
	defer r.workers.Done()
	defer func() { <-r.slots }()
	defer client.Close()

	upstream, err := r.dial("tcp")
	if err != nil {
		return
	}
	defer upstream.Close()
	stop := context.AfterFunc(r.context, func() {
		client.Close()
		upstream.Close()
	})
	defer stop()

	err = client.SetDeadline(time.Now().Add(dnsTimeout))
	if err != nil {
		return
	}

	done := make(chan struct{})
	go func() {
		io.Copy(upstream, client)
		upstream.(*net.TCPConn).CloseWrite()
		close(done)
	}()

	io.Copy(client, upstream)
	client.Close()
	<-done
}

func (r *dnsRelay) dial(network string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: dnsTimeout}
	connection, err := dialer.DialContext(r.context, network, r.upstream)
	if err != nil {
		return nil, err
	}

	err = connection.SetDeadline(time.Now().Add(dnsTimeout))
	if err != nil {
		connection.Close()
		return nil, err
	}

	return connection, nil
}
