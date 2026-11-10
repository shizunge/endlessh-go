//go:build unix

package main

import (
	"net"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func socketOption(t *testing.T, conn *net.TCPConn, level, option int) int {
	t.Helper()
	raw, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var value int
	var optionErr error
	if err := raw.Control(func(fd uintptr) {
		value, optionErr = syscall.GetsockoptInt(int(fd), level, option)
	}); err != nil {
		t.Fatal(err)
	}
	if optionErr != nil {
		t.Fatal(optionErr)
	}
	return value
}

func socketFlags(t *testing.T, conn *net.TCPConn) int {
	t.Helper()
	raw, err := conn.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var flags int
	var optionErr error
	if err := raw.Control(func(fd uintptr) {
		flags, optionErr = unix.FcntlInt(fd, unix.F_GETFL, 0)
	}); err != nil {
		t.Fatal(err)
	}
	if optionErr != nil {
		t.Fatal(optionErr)
	}
	return flags
}

func TestTCPListenerAppliesSocketOptions(t *testing.T) {
	options := tcpSocketOptions{
		keepAliveIdle:     30 * time.Second,
		keepAliveInterval: 10 * time.Second,
		keepAliveCount:    3,
		userTimeout:       time.Minute,
		readBuffer:        4096,
		writeBuffer:       4096,
	}
	listener, err := listenTCP("tcp4", "127.0.0.1:0", options)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	peer, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Close()
	tcpConn := accepted.(*net.TCPConn)
	if got := socketOption(t, tcpConn, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE); got == 0 {
		t.Errorf("SO_KEEPALIVE = %d; want enabled", got)
	}
	if got := socketOption(t, tcpConn, syscall.IPPROTO_TCP, syscall.TCP_NODELAY); got == 0 {
		t.Errorf("TCP_NODELAY = %d; want enabled", got)
	}
	if flags := socketFlags(t, tcpConn); flags&unix.O_NONBLOCK == 0 {
		t.Errorf("socket flags %#x do not include O_NONBLOCK", flags)
	}
	for _, option := range []int{syscall.SO_RCVBUF, syscall.SO_SNDBUF} {
		if got := socketOption(t, tcpConn, syscall.SOL_SOCKET, option); got > 16384 {
			t.Errorf("socket buffer option %d = %d; want at most 16384", option, got)
		}
	}
}
