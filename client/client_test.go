package client

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testConn struct {
	writeDeadline time.Time
	deadlineErr   error
	wrote         bool
}

func (*testConn) Read([]byte) (int, error) { return 0, io.EOF }
func (c *testConn) Write(b []byte) (int, error) {
	c.wrote = true
	return len(b), nil
}
func (*testConn) Close() error                    { return nil }
func (*testConn) LocalAddr() net.Addr             { return &net.TCPAddr{Port: 2222} }
func (*testConn) RemoteAddr() net.Addr            { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345} }
func (*testConn) SetDeadline(time.Time) error     { return nil }
func (*testConn) SetReadDeadline(time.Time) error { return nil }
func (c *testConn) SetWriteDeadline(deadline time.Time) error {
	c.writeDeadline = deadline
	return c.deadlineErr
}

func TestNewClientEnforcesLimitConcurrently(t *testing.T) {
	const limit = 5
	const attempts = 64
	var wg sync.WaitGroup
	accepted := make(chan *Client, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c := NewClient(&testConn{}, time.Second, limit); c != nil {
				accepted <- c
			}
		}()
	}
	wg.Wait()
	close(accepted)
	if got := len(accepted); got != limit {
		t.Errorf("accepted %d clients; want %d", got, limit)
	}
	for c := range accepted {
		c.Close()
	}
	if got := atomic.LoadInt64(&numCurrentClients); got != 0 {
		t.Errorf("client count after close = %d; want 0", got)
	}
}

func TestSendCapsWriteDeadline(t *testing.T) {
	conn := &testConn{}
	c := &Client{conn: conn, next: time.Now().Add(-time.Second), interval: time.Hour, writeTimeout: 5 * time.Second}
	before := time.Now()
	if _, err := c.Send(32); err != nil {
		t.Fatal(err)
	}
	if remaining := conn.writeDeadline.Sub(before); remaining < 0 || remaining > 6*time.Second {
		t.Fatalf("write deadline after %v; want about 5s", remaining)
	}
}

func TestSendStopsIfWriteDeadlineFails(t *testing.T) {
	conn := &testConn{deadlineErr: errors.New("deadline unavailable")}
	c := &Client{conn: conn, next: time.Now().Add(-time.Second), interval: time.Second, writeTimeout: time.Second}
	if _, err := c.Send(32); !errors.Is(err, conn.deadlineErr) {
		t.Fatalf("Send error = %v; want %v", err, conn.deadlineErr)
	}
	if conn.wrote {
		t.Fatal("Send wrote data without a write deadline")
	}
}

func TestSendTimesOutBlockedWrite(t *testing.T) {
	server, peer := net.Pipe()
	defer server.Close()
	defer peer.Close()
	c := &Client{conn: server, next: time.Now().Add(-time.Second), interval: time.Hour, writeTimeout: 50 * time.Millisecond}
	started := time.Now()
	_, err := c.Send(32)
	if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("Send error = %v; want timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("blocked write took %v; want under 1s", elapsed)
	}
}
