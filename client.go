// Copyright (C) 2021 Shizun Ge
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

package main

import (
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
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
	conn              net.Conn
	last              time.Time
	next              time.Time
	start             time.Time
	interval          time.Duration
	geoipSupplier     string
	geohash           string
	country           string
	location          string
	bytesSent         int
	prometheusEnabled bool
}

func NewClient(conn net.Conn, interval time.Duration, maxClient int64, geoipSupplier string, prometheusEnabled bool) *client {
	addr := conn.RemoteAddr().(*net.TCPAddr)
	atomic.AddInt64(&numCurrentClients, 1)
	atomic.AddInt64(&numTotalClients, 1)
	geohash, country, location, err := geohashAndLocation(addr.IP.String(), geoipSupplier)
	if err != nil {
		glog.Warningf("Failed to obatin the geohash of %v: %v.", addr.IP, err)
	}
	if prometheusEnabled {
		clientIP.With(prometheus.Labels{
			"ip":       addr.IP.String(),
			"geohash":  geohash,
			"country":  country,
			"location": location}).Inc()
	}
	glog.V(1).Infof("ACCEPT host=%v port=%v n=%v/%v\n", addr.IP, addr.Port, numCurrentClients, maxClient)
	return &client{
		conn:              conn,
		last:              time.Now(),
		next:              time.Now().Add(interval),
		start:             time.Now(),
		interval:          interval,
		geohash:           geohash,
		country:           country,
		location:          location,
		bytesSent:         0,
		prometheusEnabled: prometheusEnabled,
	}
}

func (c *client) Send(bannerMaxLength int64) error {
	defer func(c *client) {
		addr := c.conn.RemoteAddr().(*net.TCPAddr)
		millisecondsSpent := time.Now().Sub(c.last).Milliseconds()
		c.last = time.Now()
		c.next = time.Now().Add(c.interval)
		atomic.AddInt64(&numTotalMilliseconds, millisecondsSpent)
		if c.prometheusEnabled {
			clientSeconds.With(prometheus.Labels{"ip": addr.IP.String()}).Add(float64(millisecondsSpent) / 1000)
		}
	}(c)
	length := rand.Int63n(bannerMaxLength)
	bytesSent, err := c.conn.Write(randStringBytes(length))
	if err != nil {
		return err
	}
	c.bytesSent += bytesSent
	atomic.AddInt64(&numTotalBytes, int64(bytesSent))
	return nil
}

func (c *client) Close() {
	addr := c.conn.RemoteAddr().(*net.TCPAddr)
	atomic.AddInt64(&numCurrentClients, -1)
	atomic.AddInt64(&numTotalClientsClosed, 1)
	glog.V(1).Infof("CLOSE host=%v port=%v time=%v bytes=%v\n", addr.IP, addr.Port, time.Now().Sub(c.start).Seconds(), c.bytesSent)
	c.conn.Close()
}
