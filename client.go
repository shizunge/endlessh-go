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
	conn          net.Conn
	last          time.Time
	next          time.Time
	start         time.Time
	interval      time.Duration
	geoipSupplier string
	geohash       string
	country       string
	location      string
	bytes_sent    int
}

func NewClient(conn net.Conn, interval time.Duration, maxClient int64, geoipSupplier string) *client {
	addr := conn.RemoteAddr().(*net.TCPAddr)
	atomic.AddInt64(&numCurrentClients, 1)
	atomic.AddInt64(&numTotalClients, 1)
	geohash, country, location, err := geohashAndLocation(addr.IP.String(), geoipSupplier)
	if err != nil {
		glog.Warningf("Failed to obatin the geohash of %v: %v.", addr.IP, err)
	}
	clientIP.With(prometheus.Labels{
		"ip":       addr.IP.String(),
		"geohash":  geohash,
		"country":  country,
		"location": location}).Inc()
	glog.V(1).Infof("ACCEPT host=%v port=%v n=%v/%v\n", addr.IP, addr.Port, numCurrentClients, maxClient)
	return &client{
		conn:       conn,
		last:       time.Now(),
		next:       time.Now().Add(interval),
		start:      time.Now(),
		interval:   interval,
		geohash:    geohash,
		country:    country,
		location:   location,
		bytes_sent: 0,
	}
}

func (c *client) Send(bannerMaxLength int64) error {
	defer func(c *client) {
		addr := c.conn.RemoteAddr().(*net.TCPAddr)
		millisecondsSpent := time.Now().Sub(c.last).Milliseconds()
		c.last = time.Now()
		c.next = time.Now().Add(c.interval)
		atomic.AddInt64(&numTotalMilliseconds, millisecondsSpent)
		clientSeconds.With(prometheus.Labels{"ip": addr.IP.String()}).Add(float64(millisecondsSpent) / 1000)
	}(c)
	length := rand.Int63n(bannerMaxLength)
	bytes_sent, err := c.conn.Write(randStringBytes(length))
	if err != nil {
		return err
	}
	c.bytes_sent += bytes_sent
	atomic.AddInt64(&numTotalBytes, int64(bytes_sent))
	return nil
}

func (c *client) Close() {
	addr := c.conn.RemoteAddr().(*net.TCPAddr)
	atomic.AddInt64(&numCurrentClients, -1)
	atomic.AddInt64(&numTotalClientsClosed, 1)
	glog.V(1).Infof("CLOSE host=%v port=%v time=%v bytes=%v\n", addr.IP, addr.Port, time.Now().Sub(c.start).Seconds(), c.bytes_sent)
	c.conn.Close()
}
