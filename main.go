package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	numCurrentClients     int64
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

func initPrometheus(connHost, prometheusPort, prometheusEntry string) {
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
	prometheus.MustRegister(totalClients)
	prometheus.MustRegister(totalClientsClosed)
	prometheus.MustRegister(totalBytes)
	prometheus.MustRegister(totalSeconds)
	prometheus.MustRegister(clientIP)
	prometheus.MustRegister(clientSeconds)
	http.Handle("/"+prometheusEntry, promhttp.Handler())
	go func() {
		glog.Infof("Starting Prometheus on %v:%v, entry point is /%v", connHost, prometheusPort, prometheusEntry)
		http.ListenAndServe(connHost+":"+prometheusPort, nil)
	}()
}

func main() {
	intervalMs := flag.Int("interval_ms", 1000, "Message millisecond delay")
	bannerMaxLength := flag.Int64("line_length", 32, "Maximum banner line length")
	maxClients := flag.Int64("max_clients", 4096, "Maximum number of clients")
	connType := flag.String("conn_type", "tcp", "Connection type. Possible values are tcp, tcp4, tcp6")
	connHost := flag.String("host", "0.0.0.0", "Listening address")
	connPort := flag.String("port", "2222", "Listening port")
	enablePrometheus := flag.Bool("enable_prometheus", false, "Enable prometheus")
	prometheusPort := flag.String("prometheus_port", "2112", "The port for prometheus")
	prometheusEntry := flag.String("prometheus_entry", "metrics", "Entry point for prometheus")
	geoipSupplier := flag.String("geoip_supplier", "ip-api", "Supplier to obtain Geohash of IPs. Possible values are \"ip-api\", \"freegeoip\"")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %v \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *enablePrometheus {
		initPrometheus(*connHost, *prometheusPort, *prometheusEntry)
	}

	rand.Seed(time.Now().UnixNano())
	interval := time.Duration(*intervalMs) * time.Millisecond
	// Listen for incoming connections.
	if *connType == "tcp6" && *connHost == "0.0.0.0" {
		*connHost = "[::]"
	}
	l, err := net.Listen(*connType, *connHost+":"+*connPort)
	if err != nil {
		glog.Errorf("Error listening: %v", err)
		os.Exit(1)
	}
	// Close the listener when the application closes.
	defer l.Close()
	glog.Infof("Listening on %v:%v", *connHost, *connPort)

	clients := make(chan *client, *maxClients)
	go func() {
		for {
			c, more := <-clients
			if !more {
				return
			}
			if time.Now().Before(c.next) {
				time.Sleep(c.next.Sub(time.Now()))
			}
			err := c.Send(*bannerMaxLength)
			if err != nil {
				c.Close()
				continue
			}
			go func() { clients <- c }()
		}
	}()
	listener := func() {
		for {
			// Listen for an incoming connection.
			conn, err := l.Accept()
			if err != nil {
				glog.Errorf("Error accepting: %v", err)
				os.Exit(1)
			}
			// Handle connections in a new goroutine.
			for numCurrentClients >= *maxClients {
				time.Sleep(interval)
			}
			clients <- NewClient(conn, interval, *maxClients, *geoipSupplier)
		}
	}
	listener()
}
