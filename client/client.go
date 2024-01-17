// Copyright (C) 2021-2024 Shizun Ge
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package client

import (
	"math/rand"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
)

var (
	numCurrentClients int64
	letterBytes       = []byte(" abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ01234567890!@#$%^&*()-=_+[]{}|;:',./<>?")
)

func randStringBytes(n int64) []byte {
	b := make([]byte, n+1)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	b[n] = '\n'
	return b
}

type Client struct {
	conn      net.Conn
	next      time.Time
	start     time.Time
	last      time.Time
	interval  time.Duration
	bytesSent int
}

func NewClient(conn net.Conn, interval time.Duration, maxClients int64) *Client {
	for numCurrentClients >= maxClients {
		time.Sleep(interval)
	}
	atomic.AddInt64(&numCurrentClients, 1)
	addr := conn.RemoteAddr().(*net.TCPAddr)
	glog.V(1).Infof("ACCEPT host=%v port=%v n=%v/%v\n", addr.IP, addr.Port, numCurrentClients, maxClients)
	return &Client{
		conn:      conn,
		next:      time.Now().Add(interval),
		start:     time.Now(),
		last:      time.Now(),
		interval:  interval,
		bytesSent: 0,
	}
}

func (c *Client) RemoteIpAddr() string {
	return c.conn.RemoteAddr().(*net.TCPAddr).IP.String()
}

func (c *Client) LocalPort() string {
	return strconv.Itoa(c.conn.LocalAddr().(*net.TCPAddr).Port)
}

func (c *Client) Send(bannerMaxLength int64) (int, error) {
	if time.Now().Before(c.next) {
		time.Sleep(c.next.Sub(time.Now()))
	}
	c.next = time.Now().Add(c.interval)
	length := rand.Int63n(bannerMaxLength)
	bytesSent, err := c.conn.Write(randStringBytes(length))
	if err != nil {
		return 0, err
	}
	c.bytesSent += bytesSent
	return bytesSent, nil
}

func (c *Client) MillisecondsSinceLast() int64 {
	millisecondsSpent := time.Now().Sub(c.last).Milliseconds()
	c.last = time.Now()
	return millisecondsSpent
}

func (c *Client) Close() {
	addr := c.conn.RemoteAddr().(*net.TCPAddr)
	glog.V(1).Infof("CLOSE host=%v port=%v time=%v bytes=%v\n", addr.IP, addr.Port, time.Now().Sub(c.start).Seconds(), c.bytesSent)
	c.conn.Close()
	atomic.AddInt64(&numCurrentClients, -1)
}
