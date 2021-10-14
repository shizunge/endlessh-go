package main

//  echo -n "test out the server" | nc localhost 3333

import (
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
)

var letterBytes = []byte(" abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ01234567890!@#$%^&*()-=_+[]{}|;:',./<>?")

func randStringBytes(n int64) []byte {
	b := make([]byte, n+1)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	b[n] = '\n'
	return b
}

type client struct {
	conn       net.Conn
	next       time.Time
	start      time.Time
	bytes_sent int
}

func NewClient(conn net.Conn, interval time.Duration, maxClient int64) *client {
	atomic.AddInt64(&numCurrentClients, 1)
	atomic.AddUint64(&numTotalClients, 1)
	addr := conn.RemoteAddr().(*net.TCPAddr)
	glog.V(1).Infof("ACCEPT host=%v port=%v n=%v/%v\n", addr.IP, addr.Port, numCurrentClients, maxClient)
	return &client{
		conn:       conn,
		next:       time.Now().Add(interval),
		start:      time.Now(),
		bytes_sent: 0,
	}
}

func (c *client) Send(bannerMaxLength int64) error {
	length := rand.Int63n(bannerMaxLength)
	bytes_sent, err := c.conn.Write(randStringBytes(length))
	if err != nil {
		return err
	}
	c.bytes_sent += bytes_sent
	atomic.AddUint64(&numTotalBytes, uint64(bytes_sent))
	return nil
}

func (c *client) Close() {
	atomic.AddInt64(&numCurrentClients, -1)
	addr := c.conn.RemoteAddr().(*net.TCPAddr)
	glog.V(1).Infof("CLOSE host=%v port=%v time=%v bytes=%v\n", addr.IP, addr.Port, time.Now().Sub(c.start).Seconds(), c.bytes_sent)
	c.conn.Close()
}
