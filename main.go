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
	"net"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"
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

func startAccepting(maxClients int64, connType, connHost, connPort string, interval time.Duration, clients chan<- *client.Client, records chan<- metrics.RecordEntry) {
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
		}
	}()
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

type Config struct {
	IntervalMs                 int    `yaml:"interval_ms"`
	BannerMaxLength            int64  `yaml:"banner_max_length"`
	MaxClients                 int64  `yaml:"max_clients"`
	ConnType                   string `yaml:"conn_type"`
	ConnHost                   string `yaml:"conn_host"`
	ConnPort                   string `yaml:"conn_port"`
	EnablePrometheus           bool   `yaml:"enable_prometheus"`
	PrometheusHost             string `yaml:"prometheus_host"`
	PrometheusPort             string `yaml:"prometheus_port"`
	PrometheusEntry            string `yaml:"prometheus_entry"`
	PrometheusCleanUnseenSeconds int    `yaml:"prometheus_clean_unseen_seconds"`
	GeoipSupplier               string `yaml:"geoip_supplier"`
	MaxMindDbFileName           string `yaml:"max_mind_db"`
}

func parseConfigFile(path string) (Config, error) {
	config := Config{}
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func main() {

	configPath := flag.String("config-file", "", "Path to the configuration file")
	flag.Parse()

	var config Config
	if *configPath != "" {
		fileConfig, err := parseConfigFile(*configPath)
		if err != nil {
			glog.Errorf("Error parsing configuration file: %v", err)
			os.Exit(1)
		}
		config = fileConfig
	} else {
		config = Config{
			IntervalMs:                 1000,
			BannerMaxLength:            32,
			MaxClients:                 4096,
			ConnType:                   "tcp",
			ConnHost:                   "0.0.0.0",
			ConnPort:                   defaultPort,
			EnablePrometheus:           false,
			PrometheusHost:             "0.0.0.0",
			PrometheusPort:             "2112",
			PrometheusEntry:            "metrics",
			PrometheusCleanUnseenSeconds: 0,
			GeoipSupplier:               "off",
			MaxMindDbFileName:           "",
		}
	}

	intervalMs := flag.Int("interval_ms", config.IntervalMs, "Message millisecond delay")
	bannerMaxLength := flag.Int64("line_length", config.BannerMaxLength, "Maximum banner line length")
	maxClients := flag.Int64("max_clients", config.MaxClients, "Maximum number of clients")
	connType := flag.String("conn_type", config.ConnType, "Connection type. Possible values are tcp, tcp4, tcp6")
	connHost := flag.String("host", config.ConnHost, "SSH listening address") 
	flag.Var(&connPorts, "port", fmt.Sprintf("SSH listening port. You may provide multiple -port flags to listen to multiple ports. (default %q)", config.ConnPort))
	prometheusEnabled := flag.Bool("enable_prometheus", config.EnablePrometheus, "Enable prometheus")
	prometheusHost := flag.String("prometheus_host", config.PrometheusHost, "The address for prometheus")
	prometheusPort := flag.String("prometheus_port", config.PrometheusPort, "The port for prometheus")
	prometheusEntry := flag.String("prometheus_entry", config.PrometheusEntry, "Entry point for prometheus")
	prometheusCleanUnseenSeconds := flag.Int("prometheus_clean_unseen_seconds", config.PrometheusCleanUnseenSeconds, "Remove series if the IP is not seen for the given time. Set to 0 to disable. (default 0)")
	geoipSupplier := flag.String("geoip_supplier", config.GeoipSupplier, "Supplier to obtain Geohash of IPs. Possible values are \"off\", \"ip-api\", \"max-mind-db\"")
	maxMindDbFileName := flag.String("max_mind_db", config.MaxMindDbFileName, "Path to the MaxMind DB file.")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %v \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

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
		startAccepting(*maxClients, *connType, *connHost, connPort, interval, clients, records)
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
