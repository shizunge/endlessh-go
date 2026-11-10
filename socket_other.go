//go:build !linux

package main

import (
	"syscall"
	"time"
)

func tcpUserTimeoutControl(time.Duration) func(string, string, syscall.RawConn) error {
	return nil
}
