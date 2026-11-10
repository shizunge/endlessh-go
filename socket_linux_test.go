//go:build linux

package main

import (
	"net"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestTCPUserTimeoutInheritedByAcceptedSocket(t *testing.T) {
	listener, err := listenTCP("tcp4", "127.0.0.1:0", tcpSocketOptions{
		keepAliveIdle: 30 * time.Second, keepAliveInterval: 10 * time.Second,
		keepAliveCount: 3, userTimeout: time.Minute,
	})
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
	if got := socketOption(t, accepted.(*net.TCPConn), unix.IPPROTO_TCP, unix.TCP_USER_TIMEOUT); got != 60000 {
		t.Errorf("TCP_USER_TIMEOUT = %d; want 60000", got)
	}
	for _, check := range []struct{ option, want int }{
		{unix.TCP_KEEPIDLE, 30},
		{unix.TCP_KEEPINTVL, 10},
		{unix.TCP_KEEPCNT, 3},
	} {
		if got := socketOption(t, accepted.(*net.TCPConn), unix.IPPROTO_TCP, check.option); got != check.want {
			t.Errorf("TCP option %d = %d; want %d", check.option, got, check.want)
		}
	}
}
