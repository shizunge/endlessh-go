package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"
)

type tcpSocketOptions struct {
	keepAliveIdle     time.Duration
	keepAliveInterval time.Duration
	keepAliveCount    int
	userTimeout       time.Duration
	readBuffer        int
	writeBuffer       int
	writeTimeout      time.Duration
}

type tunedTCPListener struct {
	net.Listener
	readBuffer    int
	writeBuffer   int
	readWarnOnce  sync.Once
	writeWarnOnce sync.Once
}

func (l *tunedTCPListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return conn, nil
	}
	if l.readBuffer > 0 {
		if err := tcpConn.SetReadBuffer(l.readBuffer); err != nil {
			l.readWarnOnce.Do(func() { glog.Warningf("Cannot set TCP receive buffer: %v", err) })
		}
	}
	if l.writeBuffer > 0 {
		if err := tcpConn.SetWriteBuffer(l.writeBuffer); err != nil {
			l.writeWarnOnce.Do(func() { glog.Warningf("Cannot set TCP send buffer: %v", err) })
		}
	}
	return conn, nil
}

func listenTCP(network, address string, options tcpSocketOptions) (net.Listener, error) {
	config := net.ListenConfig{
		KeepAliveConfig: net.KeepAliveConfig{
			Enable:   true,
			Idle:     options.keepAliveIdle,
			Interval: options.keepAliveInterval,
			Count:    options.keepAliveCount,
		},
		Control: tcpUserTimeoutControl(options.userTimeout),
	}
	listener, err := config.Listen(context.Background(), network, address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	return &tunedTCPListener{Listener: listener, readBuffer: options.readBuffer, writeBuffer: options.writeBuffer}, nil
}
