// Copyright (C) 2021-2026 Shizun Ge
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
	"endlessh-go/client"
	"endlessh-go/geoip"
	"endlessh-go/health"
	"endlessh-go/metrics"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	proxyproto "github.com/pires/go-proxyproto"
)

func startSending(maxClients int64, bannerMaxLength int64, records chan<- metrics.RecordEntry) chan *client.Client {
	clients := make(chan *client.Client, maxClients)
	go func() {
		for {
			c, more := <-clients
			if !more {
				return
			}
			go func() {
				bytesSent, err := c.Send(bannerMaxLength)
				remoteIpAddr := c.RemoteIpAddr()
				localPort := c.LocalPort()
				millisecondsSpent := c.MillisecondsSinceLast()
				if err != nil {
					c.Close()
					records <- metrics.RecordEntry{
						RecordType:        metrics.RecordEntryTypeStop,
						IpAddr:            remoteIpAddr,
						LocalPort:         localPort,
						MillisecondsSpent: millisecondsSpent,
					}
					return
				}
				records <- metrics.RecordEntry{
					RecordType:        metrics.RecordEntryTypeSend,
					IpAddr:            remoteIpAddr,
					LocalPort:         localPort,
					MillisecondsSpent: millisecondsSpent,
					BytesSent:         bytesSent,
				}
				clients <- c
			}()
		}
	}()
	return clients
}

func startAccepting(maxClients int64, connType, connHost, connPort string, interval time.Duration, clients chan<- *client.Client, records chan<- metrics.RecordEntry, proxyProtocolEnabled bool, proxyProtocolReadHeaderTimeout int, socketOptions tcpSocketOptions) error {
	connPortInt, err := strconv.Atoi(connPort)
	if err != nil || connPortInt < 0 || connPortInt > 65535 {
		return fmt.Errorf("invalid port %q", connPort)
	}
	addr := net.JoinHostPort(connHost, connPort)
	l, err := listenTCP(connType, addr, socketOptions)
	if err != nil {
		return err
	}
	go func() {

		// Wrap the listener in a proxy protocol listener
		if proxyProtocolEnabled {
			l = &proxyproto.Listener{Listener: l, ReadHeaderTimeout: time.Duration(proxyProtocolReadHeaderTimeout) * time.Millisecond}
		}

		// Close the listener when the application closes.
		defer l.Close()

		realAddr := l.Addr().(*net.TCPAddr)
		localPortStr := strconv.Itoa(realAddr.Port)
		glog.Infof("Listening on %v:%v", connHost, realAddr.Port)
		for {
			// Listen for an incoming connection.
			conn, err := l.Accept()
			if err != nil {
				glog.Errorf("Error accepting connection from port %v: %v", connPort, err)
				os.Exit(1)
			}
			c := client.NewClient(conn, interval, maxClients)
			if c == nil {
				conn.Close()
				continue
			}
			c.SetWriteTimeout(socketOptions.writeTimeout)
			remoteIpAddr := c.RemoteIpAddr()
			records <- metrics.RecordEntry{
				RecordType: metrics.RecordEntryTypeStart,
				IpAddr:     remoteIpAddr,
				LocalPort:  localPortStr,
			}
			clients <- c
		}
	}()
	return nil
}

type arrayStrings []string

func (a *arrayStrings) String() string {
	return strings.Join(*a, ", ")
}

func (a *arrayStrings) Set(value string) error {
	*a = append(*a, value)
	return nil
}

const defaultPort = "2222"

var connPorts arrayStrings

func main() {
	// Core SSH server flags
	connHost := flag.String("host", "0.0.0.0", "SSH listening address")
	flag.Var(&connPorts, "port", fmt.Sprintf("SSH listening port. You may provide multiple -port flags to listen to multiple ports. (default %q)", defaultPort))
	connType := flag.String("conn_type", "tcp", "Connection type. Possible values are tcp, tcp4, tcp6")
	intervalMs := flag.Int("interval_ms", 1000, "Message millisecond delay")
	bannerMaxLength := flag.Int64("line_length", 32, "Maximum banner line length")
	maxClients := flag.Int64("max_clients", 4096, "Maximum number of clients")
	tcpKeepAliveIdleSeconds := flag.Int("tcp_keepalive_idle_seconds", 30, "Seconds of TCP inactivity before the first keepalive probe")
	tcpKeepAliveIntervalSeconds := flag.Int("tcp_keepalive_interval_seconds", 10, "Seconds between TCP keepalive probes")
	tcpKeepAliveCount := flag.Int("tcp_keepalive_count", 3, "Number of unanswered TCP keepalive probes before closing")
	tcpUserTimeoutMs := flag.Int("tcp_user_timeout_ms", 60000, "Linux: maximum milliseconds of unacknowledged TCP data; 0 uses the system default")
	tcpReadBufferBytes := flag.Int("tcp_read_buffer_bytes", 16384, "TCP receive buffer size in bytes; 0 uses the system default")
	tcpWriteBufferBytes := flag.Int("tcp_write_buffer_bytes", 16384, "TCP send buffer size in bytes; 0 uses the system default")
	tcpWriteTimeoutMs := flag.Int("tcp_write_timeout_ms", 5000, "Maximum milliseconds a TCP write may wait for a slow peer")

	// PROXY protocol flags
	proxyProtocolEnabled := flag.Bool("proxy_protocol_enabled", false, "Enable PROXY protocol support. This causes the server to expect PROXY protocol headers on incoming connections.")
	proxyProtocolReadHeaderTimeout := flag.Int("proxy_protocol_read_header_timeout_ms", 200, "Timeout for reading the PROXY protocol header in milliseconds. If the connection does not send a valid PROXY protocol header in this time, the header is ignored.")

	// Prometheus metrics flags
	prometheusEnabledOld := flag.Bool("enable_prometheus", false, "Enable prometheus (deprecated, use prometheus_enabled)")
	prometheusEnabledNew := flag.Bool("prometheus_enabled", false, "Enable prometheus")
	prometheusHost := flag.String("prometheus_host", "0.0.0.0", "The address for prometheus")
	prometheusPort := flag.String("prometheus_port", "2112", "The port for prometheus")
	prometheusEntry := flag.String("prometheus_entry", "metrics", "Entry point for prometheus")
	prometheusCleanUnseenSeconds := flag.Int("prometheus_clean_unseen_seconds", 3600, "Remove per-IP series if the IP is not seen for the given number of seconds. Set to 0 to disable.")
	prometheusMaxTrackedIPs := flag.Int("prometheus_max_tracked_ips", 10000, "Maximum number of IPs retained in per-IP Prometheus metrics")

	// GeoIP flags
	geoipSupplier := flag.String("geoip_supplier", "off", "Supplier to obtain Geohash of IPs. Possible values are \"off\", \"ip-api\", \"max-mind-db\"")
	maxMindDbFileName := flag.String("max_mind_db", "", "Path to the MaxMind DB file.")

	// Healthcheck flags
	healthcheckEnabled := flag.Bool("healthcheck_enabled", false, "Enable healthcheck")
	healthcheckHost := flag.String("healthcheck_host", health.DefaultHost, "The address for healthcheck.")
	healthcheckPort := flag.String("healthcheck_port", health.DefaultPort, "HTTP port for healthcheck; Serves JSON with status and uptime at /health.")
	healthcheck := flag.Bool("healthcheck", false, "Perform healthcheck and exit. GET healthcheck_host:healthcheck_port/health and exit 1 if status is not ok or timeout is exceeded.")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %v \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if *intervalMs <= 0 || *bannerMaxLength <= 0 || *maxClients <= 0 {
		glog.Fatal("interval_ms, line_length, and max_clients must be positive")
	}
	if *prometheusCleanUnseenSeconds < 0 {
		glog.Fatal("prometheus_clean_unseen_seconds must not be negative")
	}
	if time.Duration(*prometheusCleanUnseenSeconds) > time.Duration(1<<63-1)/time.Second {
		glog.Fatal("prometheus_clean_unseen_seconds is too large")
	}
	if *prometheusMaxTrackedIPs <= 0 {
		glog.Fatal("prometheus_max_tracked_ips must be positive")
	}
	if time.Duration(*intervalMs) > time.Duration(1<<63-1)/time.Millisecond {
		glog.Fatal("interval_ms is too large")
	}
	if *proxyProtocolEnabled && (*proxyProtocolReadHeaderTimeout <= 0 || time.Duration(*proxyProtocolReadHeaderTimeout) > time.Duration(1<<63-1)/time.Millisecond) {
		glog.Fatal("proxy_protocol_read_header_timeout_ms must be a positive, representable duration")
	}
	if *tcpKeepAliveIdleSeconds < 1 || *tcpKeepAliveIdleSeconds > 86400 || *tcpKeepAliveIntervalSeconds < 1 || *tcpKeepAliveIntervalSeconds > 3600 || *tcpKeepAliveCount < 1 || *tcpKeepAliveCount > 127 {
		glog.Fatal("invalid TCP keepalive settings")
	}
	if *tcpUserTimeoutMs < 0 || *tcpUserTimeoutMs > 2147483647 {
		glog.Fatal("tcp_user_timeout_ms must be between 0 and 2147483647")
	}
	if (*tcpReadBufferBytes != 0 && (*tcpReadBufferBytes < 4096 || *tcpReadBufferBytes > 1<<20)) || (*tcpWriteBufferBytes != 0 && (*tcpWriteBufferBytes < 4096 || *tcpWriteBufferBytes > 1<<20)) {
		glog.Fatal("TCP buffer sizes must be 0 or between 4096 and 1048576 bytes")
	}
	if *tcpWriteTimeoutMs < 1 || *tcpWriteTimeoutMs > 60000 {
		glog.Fatal("tcp_write_timeout_ms must be between 1 and 60000")
	}

	if *healthcheck {
		health.SetupHealthcheck(true, false, *connType, healthcheckHost, healthcheckPort)
		return
	}

	prometheusEnabled := *prometheusEnabledNew
	prometheusEnableSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "prometheus_enabled" {
			prometheusEnableSet = true
		}
	})
	if !prometheusEnableSet {
		prometheusEnabled = *prometheusEnabledOld
	}

	if prometheusEnabled {
		if *connType == "tcp6" && *prometheusHost == "0.0.0.0" {
			*prometheusHost = "::"
		}
		if *prometheusPort == "" {
			*prometheusPort = "0"
		}
		if err := metrics.InitPrometheus(*prometheusHost, *prometheusPort, *prometheusEntry); err != nil {
			glog.Fatal(err)
		}
	}

	records := metrics.StartRecording(*maxClients, prometheusEnabled, *prometheusCleanUnseenSeconds, *prometheusMaxTrackedIPs,
		geoip.GeoOption{
			GeoipSupplier:     *geoipSupplier,
			MaxMindDbFileName: *maxMindDbFileName,
		})
	clients := startSending(*maxClients, *bannerMaxLength, records)

	interval := time.Duration(*intervalMs) * time.Millisecond
	socketOptions := tcpSocketOptions{
		keepAliveIdle:     time.Duration(*tcpKeepAliveIdleSeconds) * time.Second,
		keepAliveInterval: time.Duration(*tcpKeepAliveIntervalSeconds) * time.Second,
		keepAliveCount:    *tcpKeepAliveCount,
		userTimeout:       time.Duration(*tcpUserTimeoutMs) * time.Millisecond,
		readBuffer:        *tcpReadBufferBytes,
		writeBuffer:       *tcpWriteBufferBytes,
		writeTimeout:      time.Duration(*tcpWriteTimeoutMs) * time.Millisecond,
	}
	// Listen for incoming connections.
	if *connType == "tcp6" && *connHost == "0.0.0.0" {
		*connHost = "::"
	}
	if len(connPorts) == 0 {
		connPorts = append(connPorts, defaultPort)
	}
	for _, connPort := range connPorts {
		if err := startAccepting(*maxClients, *connType, *connHost, connPort, interval, clients, records, *proxyProtocolEnabled, *proxyProtocolReadHeaderTimeout, socketOptions); err != nil {
			glog.Fatal(err)
		}
	}
	health.SetupHealthcheck(false, *healthcheckEnabled, *connType, healthcheckHost, healthcheckPort)
	for {
		if !prometheusEnabled || *prometheusCleanUnseenSeconds <= 0 {
			time.Sleep(time.Duration(1<<63 - 1))
		} else {
			time.Sleep(time.Second * time.Duration(60))
			records <- metrics.RecordEntry{
				RecordType: metrics.RecordEntryTypeClean,
			}
		}
	}
}
