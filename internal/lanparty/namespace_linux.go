package lanparty

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

const namespaceTimeout = 2 * time.Second

type Namespace struct {
	fd       netns.NsHandle
	listener *net.UnixListener
	done     chan struct{}
}

func NewNamespace() (*Namespace, error) {
	type result struct {
		fd  netns.NsHandle
		err error
	}

	ready := make(chan result, 1)

	go func() {
		runtime.LockOSThread()

		original, err := netns.Get()
		if err != nil {
			runtime.UnlockOSThread()
			ready <- result{err: err}
			return
		}
		defer original.Close()

		fd, nErr := netns.New()

		err = netns.Set(original)
		if err != nil {
			fd.Close()
			ready <- result{err: fmt.Errorf("restore host namespace: %w", err)}
			// Returning while locked discards the thread instead of reusing it.
			return
		}

		runtime.UnlockOSThread()
		ready <- result{fd: fd, err: nErr}
	}()

	r := <-ready
	if r.err != nil {
		return nil, fmt.Errorf("create network namespace (requires CAP_SYS_ADMIN): %w", r.err)
	}

	return &Namespace{fd: r.fd}, nil
}

func namespaceAddress() *net.UnixAddr {
	return &net.UnixAddr{Name: fmt.Sprintf("@lanparty.netns.%d", os.Getuid()), Net: "unixpacket"}
}

func checkNamespacePeer(conn *net.UnixConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	var credentials *unix.Ucred
	var peerErr error

	err = raw.Control(func(fd uintptr) {
		credentials, peerErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return err
	}
	if peerErr != nil {
		return peerErr
	}

	if credentials.Uid != uint32(os.Getuid()) {
		return errors.New("namespace socket belongs to another user")
	}

	return nil
}

// Publish makes a fully initialized session available to lanparty exec.
func (n *Namespace) Publish() error {
	listener, err := net.ListenUnix("unixpacket", namespaceAddress())
	if err != nil {
		return fmt.Errorf("publish namespace (only one --namespace session per user): %w", err)
	}

	n.listener = listener
	n.done = make(chan struct{})
	go func() {
		defer close(n.done)
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}

			n.sendNamespace(conn)
		}
	}()
	return nil
}

func (n *Namespace) sendNamespace(conn *net.UnixConn) {
	defer conn.Close()

	err := checkNamespacePeer(conn)
	if err != nil {
		return
	}

	err = conn.SetWriteDeadline(time.Now().Add(namespaceTimeout))
	if err != nil {
		return
	}

	// A single packet carries the namespace descriptor, with no request protocol.
	conn.WriteMsgUnix([]byte{1}, unix.UnixRights(int(n.fd)), nil)
}

func (n *Namespace) Close() {
	if n.listener != nil {
		n.listener.Close()
		<-n.done
	}

	n.fd.Close()
}

// EnterNamespace changes the calling thread only; callers must lock it until exec.
// An absent session leaves the thread in its original network namespace.
func EnterNamespace() error {
	conn, err := net.DialUnix("unixpacket", nil, namespaceAddress())
	if errors.Is(err, unix.ECONNREFUSED) || errors.Is(err, unix.ENOENT) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("find namespace session: %w", err)
	}
	defer conn.Close()

	err = checkNamespacePeer(conn)
	if err != nil {
		return err
	}

	err = conn.SetReadDeadline(time.Now().Add(namespaceTimeout))
	if err != nil {
		return err
	}

	var payload [1]byte
	oob := make([]byte, unix.CmsgSpace(4))

	n, oobn, flags, _, err := conn.ReadMsgUnix(payload[:], oob)
	if err != nil {
		return fmt.Errorf("receive namespace: %w", err)
	}

	messages, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return fmt.Errorf("parse namespace descriptor: %w", err)
	}

	var fds []int
	defer func() {
		for _, fd := range fds {
			unix.Close(fd)
		}
	}()

	for i := range messages {
		rights, err := unix.ParseUnixRights(&messages[i])
		if err != nil {
			return fmt.Errorf("parse namespace rights: %w", err)
		}

		fds = append(fds, rights...)
	}

	if n != 1 || payload[0] != 1 || flags&(unix.MSG_CTRUNC|unix.MSG_TRUNC) != 0 || len(fds) != 1 {
		return errors.New("invalid namespace descriptor packet")
	}

	err = unix.Setns(fds[0], unix.CLONE_NEWNET)
	if err != nil {
		return fmt.Errorf("enter network namespace (requires CAP_SYS_ADMIN): %w", err)
	}

	return nil
}
