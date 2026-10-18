package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/rafalb8/lanparty/internal/lanparty"
	"golang.org/x/sys/unix"
)

func execCommand(args []string) error {
	args, err := prepareCommand(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	command, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}

	runtime.LockOSThread()

	// Do not unlock on failure: main exits, and no other goroutine may reuse this thread.
	err = lanparty.EnterNamespace()
	if err != nil {
		return err
	}

	err = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0)
	if err != nil && !errors.Is(err, unix.EINVAL) {
		return fmt.Errorf("clear ambient capabilities: %w", err)
	}

	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var capabilities [2]unix.CapUserData

	err = unix.Capset(&header, &capabilities[0])
	if err != nil {
		return fmt.Errorf("clear capabilities: %w", err)
	}

	return unix.Exec(command, args, os.Environ())
}
