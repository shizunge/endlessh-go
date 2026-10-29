//go:build linux

package main

import (
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func tcpUserTimeoutControl(timeout time.Duration) func(string, string, syscall.RawConn) error {
	if timeout <= 0 {
		return nil
	}
	return func(_, _ string, raw syscall.RawConn) error {
		var optionErr error
		if err := raw.Control(func(fd uintptr) {
			optionErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_USER_TIMEOUT, int(timeout/time.Millisecond))
		}); err != nil {
			return err
		}
		return optionErr
	}
}
