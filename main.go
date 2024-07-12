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

package main

import (
	"endlessh-go/client"
	"endlessh-go/geoip"
	"endlessh-go/metrics"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/golang/glog"
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
				clients <- c
				records <- metrics.RecordEntry{
					RecordType:        metrics.RecordEntryTypeSend,
					IpAddr:            remoteIpAddr,
					LocalPort:         localPort,
					MillisecondsSpent: millisecondsSpent,
					BytesSent:         bytesSent,
				}
			}()
		}
	}()
	return clients
}

func startAccepting(maxClients int64, connType, connHost, connPort string, interval time.Duration, clients chan<- *client.Client, records chan<- metrics.RecordEntry, abuseipdbeEnabled bool, abuseIpdbApiKey string) {
	go func() {
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
			c := client.NewClient(conn, interval, maxClients)
			remoteIpAddr := c.RemoteIpAddr()
			records <- metrics.RecordEntry{
				RecordType: metrics.RecordEntryTypeStart,
				IpAddr:     remoteIpAddr,
				LocalPort:  connPort,
			}
			clients <- c
			go reportIPToAbuseIPDB(remoteIpAddr, records, abuseipdbeEnabled, abuseIpdbApiKey)
		}
	}()
}

func reportIPToAbuseIPDB(ip string, records chan<- metrics.RecordEntry, abuseipdbeEnabled bool, abuseIpdbApiKey string) {
	if !abuseipdbeEnabled {
		return
	}
	if isCached(ip) {
		glog.V(1).Infof("IP is already cached, skipping report")
		records <- metrics.RecordEntry{
			RecordType: metrics.RecordEntryTypeReport,
			IpAddr:     ip,
			Message:    "IP is already cached, skipping report",
		}
		return
	}
	appendToReportedIPs(ip) // Cache the IP before possibly early exiting due to API key issues

	var apiKey string
	if abuseIpdbApiKey != "" {
		apiKey = abuseIpdbApiKey
	} else {
		glog.V(1).Infof("AbuseIPDB API key not set, skipping report")
		records <- metrics.RecordEntry{
			RecordType: metrics.RecordEntryTypeReport,
			IpAddr:     ip,
			Message:    "AbuseIPDB API key not set, skipping report",
		}
		return
	}

	// Format the timestamp in ISO 8601 format, including the timezone (Z for UTC)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	reportURL := fmt.Sprintf("https://api.abuseipdb.com/api/v2/report?ip=%s&categories=18,22&comment=SSH honeypot connection attempt.&timestamp=%s", ip, timestamp)
	req, err := http.NewRequest("POST", reportURL, nil)
	if err != nil {
		glog.V(1).Infof("Error creating request: %v", err)
		records <- metrics.RecordEntry{
			RecordType: metrics.RecordEntryTypeReport,
			IpAddr:     ip,
			Message:    fmt.Sprintf("Error creating request: %v", err),
		}
		return
	}

	req.Header.Set("Key", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		records <- metrics.RecordEntry{
			RecordType: metrics.RecordEntryTypeReport,
			IpAddr:     ip,
			Message:    fmt.Sprintf("Error making request: %v", err),
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnprocessableEntity {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			glog.V(1).Infof("Error reading response body: %v", err)
		} else {
			glog.V(1).Infof("Unprocessable Entity response from AbuseIPDB: %s", body)
		}
		records <- metrics.RecordEntry{
			RecordType: metrics.RecordEntryTypeReport,
			IpAddr:     ip,
			Message:    fmt.Sprintf("Unprocessable Entity response from AbuseIPDB: %s", resp.Status),
		}
		return
	}

	glog.V(1).Infof("Reported IP to AbuseIPDB: %s", resp.Status)
	records <- metrics.RecordEntry{
		RecordType: metrics.RecordEntryTypeReport,
		IpAddr:     ip,
		Message:    fmt.Sprintf("Reported IP to AbuseIPDB: %s", resp.Status),
	}
}

func setupCache() {
	config := &ristretto.Config{
		NumCounters: 1e7,     // NumCounters is 10x the number of items you expect to keep in the cache when full.
		MaxCost:     1 << 30, // Maximum cost of cache (e.g., bytes if the cost is measured in bytes).
		BufferItems: 64,      // Number of keys per Get buffer.
	}

	var err error
	cache, err = ristretto.NewCache(config)
	if err != nil {
		glog.Fatalf("Failed to create cache: %v", err)
	}
}

func isCached(ip string) bool {
	_, found := cache.Get(ip)
	return found
}

func appendToReportedIPs(ip string) {
	cache.Set(ip, struct{}{}, 1)
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

var cache *ristretto.Cache
var connPorts arrayStrings

func main() {
	setupCache()

	abuseIPDBApiKeyFlag := flag.String("abuse_ipdb_api_key", "", "AbuseIPDB API key")
	intervalMs := flag.Int("interval_ms", 1000, "Message millisecond delay")
	bannerMaxLength := flag.Int64("line_length", 32, "Maximum banner line length")
	maxClients := flag.Int64("max_clients", 4096, "Maximum number of clients")
	connType := flag.String("conn_type", "tcp", "Connection type. Possible values are tcp, tcp4, tcp6")
	connHost := flag.String("host", "0.0.0.0", "SSH listening address")
	flag.Var(&connPorts, "port", fmt.Sprintf("SSH listening port. You may provide multiple -port flags to listen to multiple ports. (default %q)", defaultPort))
	abuseipdbeEnabled := flag.Bool("enable_abuseipdb", false, "Enable AbuseIPDB reporting")
	prometheusEnabled := flag.Bool("enable_prometheus", false, "Enable prometheus")
	prometheusHost := flag.String("prometheus_host", "0.0.0.0", "The address for prometheus")
	prometheusPort := flag.String("prometheus_port", "2112", "The port for prometheus")
	prometheusEntry := flag.String("prometheus_entry", "metrics", "Entry point for prometheus")
	prometheusCleanUnseenSeconds := flag.Int("prometheus_clean_unseen_seconds", 0, "Remove series if the IP is not seen for the given time. Set to 0 to disable. (default 0)")
	geoipSupplier := flag.String("geoip_supplier", "off", "Supplier to obtain Geohash of IPs. Possible values are \"off\", \"ip-api\", \"max-mind-db\"")
	maxMindDbFileName := flag.String("max_mind_db", "", "Path to the MaxMind DB file.")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %v \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	var apiKey string
	if *abuseIPDBApiKeyFlag != "" {
		apiKey = *abuseIPDBApiKeyFlag
	} else {
		abuseIPDBApiKeyFile := os.Getenv("ABUSE_IPDB_API_KEY_FILE")
		if abuseIPDBApiKeyFile != "" {
			key, err := os.ReadFile(abuseIPDBApiKeyFile)
			if err != nil {
				glog.Warningf("Error reading API key file: %v", err)
			} else {
				apiKey = strings.TrimSpace(string(key))
			}
		} else {
			glog.Warning("Neither abuse_ipdb_api_key flag nor ABUSE_IPDB_API_KEY_FILE environment variable is set. AbuseIPDB reporting will be disabled.")
		}
	}

	if *prometheusEnabled {
		if *connType == "tcp6" && *prometheusHost == "0.0.0.0" {
			*prometheusHost = "[::]"
		}
		metrics.InitPrometheus(*prometheusHost, *prometheusPort, *prometheusEntry)
	}

	records := metrics.StartRecording(*maxClients, *prometheusEnabled, *prometheusCleanUnseenSeconds,
		geoip.GeoOption{
			GeoipSupplier:     *geoipSupplier,
			MaxMindDbFileName: *maxMindDbFileName,
		})
	clients := startSending(*maxClients, *bannerMaxLength, records)

	interval := time.Duration(*intervalMs) * time.Millisecond
	// Listen for incoming connections.
	if *connType == "tcp6" && *connHost == "0.0.0.0" {
		*connHost = "[::]"
	}
	if len(connPorts) == 0 {
		connPorts = append(connPorts, defaultPort)
	}
	for _, connPort := range connPorts {
		startAccepting(*maxClients, *connType, *connHost, connPort, interval, clients, records, *abuseipdbeEnabled, apiKey)
	}
	for {
		if *prometheusCleanUnseenSeconds <= 0 {
			time.Sleep(time.Duration(1<<63 - 1))
		} else {
			time.Sleep(time.Second * time.Duration(60))
			records <- metrics.RecordEntry{
				RecordType: metrics.RecordEntryTypeClean,
			}
		}
	}
}
