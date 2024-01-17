// Copyright (C) 2021-2023 Shizun Ge
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
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	numTotalClients       int64
	numTotalClientsClosed int64
	numTotalBytes         int64
	numTotalMilliseconds  int64
	totalClients          prometheus.CounterFunc
	totalClientsClosed    prometheus.CounterFunc
	totalBytes            prometheus.CounterFunc
	totalSeconds          prometheus.CounterFunc
	clientIP              *prometheus.CounterVec
	clientSeconds         *prometheus.CounterVec
)

func initPrometheus(prometheusHost, prometheusPort, prometheusEntry string) {
	totalClients = prometheus.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "endlessh_client_open_count_total",
			Help: "Total number of clients that tried to connect to this host.",
		}, func() float64 {
			return float64(numTotalClients)
		},
	)
	totalClientsClosed = prometheus.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "endlessh_client_closed_count_total",
			Help: "Total number of clients that stopped connecting to this host.",
		}, func() float64 {
			return float64(numTotalClientsClosed)
		},
	)
	totalBytes = prometheus.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "endlessh_sent_bytes_total",
			Help: "Total bytes sent to clients that tried to connect to this host.",
		}, func() float64 {
			return float64(numTotalBytes)
		},
	)
	totalSeconds = prometheus.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "endlessh_trapped_time_seconds_total",
			Help: "Total seconds clients spent on endlessh.",
		}, func() float64 {
			return float64(numTotalMilliseconds) / 1000
		},
	)
	clientIP = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endlessh_client_open_count",
			Help: "Number of connections of clients.",
		},
		[]string{"ip", "geohash", "country", "location"},
	)
	clientSeconds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endlessh_client_trapped_time_seconds",
			Help: "Seconds a client spends on endlessh.",
		},
		[]string{"ip"},
	)
	promReg := prometheus.NewRegistry()
	promReg.MustRegister(totalClients)
	promReg.MustRegister(totalClientsClosed)
	promReg.MustRegister(totalBytes)
	promReg.MustRegister(totalSeconds)
	promReg.MustRegister(clientIP)
	promReg.MustRegister(clientSeconds)
	handler := promhttp.HandlerFor(promReg, promhttp.HandlerOpts{EnableOpenMetrics: true})
	http.Handle("/"+prometheusEntry, handler)
	go func() {
		glog.Infof("Starting Prometheus on %v:%v, entry point is /%v", prometheusHost, prometheusPort, prometheusEntry)
		http.ListenAndServe(prometheusHost+":"+prometheusPort, nil)
	}()
}

const (
	recordTypeStart = iota
	recordTypeSend  = iota
	recordTypeStop  = iota
)

type recordEntry struct {
	RecordType        int
	IpAddr            string
	BytesSent         int
	MillisecondsSpent int64
}

func startRecording(maxClients int64, prometheusEnabled bool, geoOption GeoOption) chan recordEntry {
	records := make(chan recordEntry, maxClients)
	go func() {
		for {
			r, more := <-records
			if !more {
				return
			}
			if !prometheusEnabled {
				continue
			}
			switch r.RecordType {
			case recordTypeStart:
				geohash, country, location, err := geohashAndLocation(r.IpAddr, geoOption)
				if err != nil {
					glog.Warningf("Failed to obatin the geohash of %v: %v.", r.IpAddr, err)
				}
				clientIP.With(prometheus.Labels{
					"ip":       r.IpAddr,
					"geohash":  geohash,
					"country":  country,
					"location": location}).Inc()
				atomic.AddInt64(&numTotalClients, 1)
			case recordTypeSend:
				clientSeconds.With(prometheus.Labels{"ip": r.IpAddr}).Add(float64(r.MillisecondsSpent) / 1000)
				atomic.AddInt64(&numTotalBytes, int64(r.BytesSent))
				atomic.AddInt64(&numTotalMilliseconds, r.MillisecondsSpent)
			case recordTypeStop:
				atomic.AddInt64(&numTotalClientsClosed, 1)
			}
		}
	}()
	return records
}

func startSending(maxClients int64, bannerMaxLength int64, records chan<- recordEntry) chan *Client {
	clients := make(chan *Client, maxClients)
	go func() {
		for {
			c, more := <-clients
			if !more {
				return
			}
			go func() {
				bytesSent, err := c.Send(bannerMaxLength)
				ipAddr := c.IpAddr()
				if err != nil {
					c.Close()
					records <- recordEntry{
						RecordType: recordTypeStop,
						IpAddr:     ipAddr,
					}
					return
				}
				millisecondsSpent := c.MillisecondsSinceLast()
				clients <- c
				records <- recordEntry{
					RecordType:        recordTypeSend,
					IpAddr:            ipAddr,
					BytesSent:         bytesSent,
					MillisecondsSpent: millisecondsSpent,
				}
			}()
		}
	}()
	return clients
}

func startAccepting(maxClients int64, connType, connHost, connPort string, interval time.Duration, clients chan<- *Client, records chan<- recordEntry) {
	l, err := net.Listen(connType, connHost+":"+connPort)
	if err != nil {
		glog.Errorf("Error listening: %v", err)
		os.Exit(1)
	}
	// Close the listener when the application closes.
	defer l.Close()
	glog.Infof("Listening on %v:%v", connHost, connPort)
	for {
		// Listen for an incoming connection.
		conn, err := l.Accept()
		if err != nil {
			glog.Errorf("Error accepting connection from port %v: %v", connPort, err)
			os.Exit(1)
		}
		c := NewClient(conn, interval, maxClients)
		ipAddr := c.IpAddr()
		records <- recordEntry{
			RecordType: recordTypeStart,
			IpAddr:     ipAddr,
		}
		clients <- c
	}
}

func main() {
	intervalMs := flag.Int("interval_ms", 1000, "Message millisecond delay")
	bannerMaxLength := flag.Int64("line_length", 32, "Maximum banner line length")
	maxClients := flag.Int64("max_clients", 4096, "Maximum number of clients")
	connType := flag.String("conn_type", "tcp", "Connection type. Possible values are tcp, tcp4, tcp6")
	connHost := flag.String("host", "0.0.0.0", "SSH listening address")
	connPort := flag.String("port", "2222", "SSH listening port")
	prometheusEnabled := flag.Bool("enable_prometheus", false, "Enable prometheus")
	prometheusHost := flag.String("prometheus_host", "0.0.0.0", "The address for prometheus")
	prometheusPort := flag.String("prometheus_port", "2112", "The port for prometheus")
	prometheusEntry := flag.String("prometheus_entry", "metrics", "Entry point for prometheus")
	geoipSupplier := flag.String("geoip_supplier", "off", "Supplier to obtain Geohash of IPs. Possible values are \"off\", \"ip-api\", \"max-mind-db\"")
	maxMindDbFileName := flag.String("max_mind_db", "", "Path to the MaxMind DB file.")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %v \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *prometheusEnabled {
		if *connType == "tcp6" && *prometheusHost == "0.0.0.0" {
			*prometheusHost = "[::]"
		}
		initPrometheus(*prometheusHost, *prometheusPort, *prometheusEntry)
	}

	records := startRecording(*maxClients, *prometheusEnabled, GeoOption{
		GeoipSupplier:     *geoipSupplier,
		MaxMindDbFileName: *maxMindDbFileName,
	})
	clients := startSending(*maxClients, *bannerMaxLength, records)

	interval := time.Duration(*intervalMs) * time.Millisecond
	// Listen for incoming connections.
	if *connType == "tcp6" && *connHost == "0.0.0.0" {
		*connHost = "[::]"
	}
	go startAccepting(*maxClients, *connType, *connHost, *connPort, interval, clients, records)
	for {
		time.Sleep(time.Duration(1<<63 - 1))
	}
}
