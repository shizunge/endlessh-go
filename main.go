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
	"context"
	"endlessh-go/app"
	"endlessh-go/client"
	"endlessh-go/geoip"
	"endlessh-go/health"
	"endlessh-go/metrics"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/golang/glog"
	proxyproto "github.com/pires/go-proxyproto"
	"github.com/urfave/cli/v3"
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

func startAccepting(maxClients int64, connType, connHost string, connPort uint16, interval time.Duration, clients chan<- *client.Client, records chan<- metrics.RecordEntry, proxyProtocolEnabled bool, proxyProtocolReadHeaderTimeout time.Duration) {
	go func() {
		addr := fmt.Sprintf("%s:%d", connHost, connPort)
		l, err := net.Listen(connType, addr)
		if err != nil {
			glog.Errorf("Error listening: %v", err)
			os.Exit(1)
		}

		// Wrap the listener in a proxy protocol listener
		if proxyProtocolEnabled {
			l = &proxyproto.Listener{Listener: l, ReadHeaderTimeout: proxyProtocolReadHeaderTimeout}
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
			remoteIpAddr := c.RemoteIpAddr()
			records <- metrics.RecordEntry{
				RecordType: metrics.RecordEntryTypeStart,
				IpAddr:     remoteIpAddr,
				LocalPort:  localPortStr,
			}
			clients <- c
		}
	}()
}

func action(ctx context.Context, cmd *cli.Command) error {
	// Core SSH server flags
	connHost := cmd.String(app.FlagHostAddr)
	connPorts := cmd.Uint16Slice(app.FlagHostPort)
	connType := cmd.String(app.FlagConnType)
	interval := app.CurrentOrDeprecated(cmd,
		cmd.Duration(app.FlagMsgDelay),
		app.FlagMsgDelayLegacy,
		func(c *cli.Command, k string) time.Duration {
			return time.Duration(cmd.Int(k)) * time.Millisecond
		})
	bannerMaxLength := cmd.Int64(app.FlagLineLength)
	maxClients := cmd.Int64(app.FlagMaxClients)

	// PROXY protocol flags
	proxyProtocolEnabled := cmd.Bool(app.FlagProxyEnable)
	proxyProtocolReadHeaderTimeout := app.CurrentOrDeprecated(cmd,
		cmd.Duration(app.FlagProxyTimeout),
		app.FlagProxyTimeoutLegacy,
		func(c *cli.Command, k string) time.Duration {
			return time.Duration(cmd.Int(k)) * time.Millisecond
		})

	// Prometheus metrics flags
	prometheusEnabled := app.CurrentOrDeprecated(cmd,
		cmd.Bool(app.FlagPrometheusEnable),
		app.FlagPrometheusEnableLegacy,
		(*cli.Command).Bool)
	prometheusHost := cmd.String(app.FlagPrometheusHost)
	prometheusPort := cmd.Uint16(app.FlagPrometheusPort)
	prometheusEntry := cmd.String(app.FlagPrometheusEntry)
	prometheusCleanUnseenDelay := app.CurrentOrDeprecated(cmd,
		cmd.Duration(app.FlagPrometheusCleanDelay),
		app.FlagPrometheusCleanDelayLegacy,
		func(c *cli.Command, k string) time.Duration {
			return time.Duration(cmd.Int(k)) * time.Second
		})

	// GeoIP flags
	geoipSupplier := cmd.String(app.FlagGeoIPSupplier)
	maxMindDbFileName := cmd.String(app.FlagGeoIPMaxMind)

	// Healthcheck flags
	healthcheckEnabled := cmd.Bool(app.FlagHealthCheckEnable)
	healthcheckHost := cmd.String(app.FlagHealthCheckHost)
	healthcheckPort := cmd.Uint16(app.FlagHealthCheckPort)
	healthcheck := cmd.Bool(app.FlagHealthCheckOneShot)

	health.SetupHealthcheck(healthcheck, healthcheckEnabled, connType, healthcheckHost, healthcheckPort)

	if prometheusEnabled {
		if connType == "tcp6" && prometheusHost == "0.0.0.0" {
			prometheusHost = "[::]"
		}
		if prometheusPort == 0 {
			l, err := net.Listen("tcp", prometheusHost+":0")
			if err != nil {
				glog.Fatalf("Failed to pick a free Prometheus port: %v", err)
			}
			actualPort := l.Addr().(*net.TCPAddr).Port
			prometheusPort = uint16(actualPort)
			l.Close()
		}
		metrics.InitPrometheus(prometheusHost, prometheusPort, prometheusEntry)
	}

	records := metrics.StartRecording(maxClients, prometheusEnabled, prometheusCleanUnseenDelay,
		geoip.GeoOption{
			GeoipSupplier:     geoipSupplier,
			MaxMindDbFileName: maxMindDbFileName,
		})
	clients := startSending(maxClients, bannerMaxLength, records)

	// Listen for incoming connections.
	if connType == "tcp6" && connHost == "0.0.0.0" {
		connHost = "[::]"
	}
	for _, connPort := range connPorts {
		startAccepting(maxClients, connType, connHost, connPort, interval, clients, records, proxyProtocolEnabled, proxyProtocolReadHeaderTimeout)
	}
	for {
		if prometheusCleanUnseenDelay <= 0 {
			time.Sleep(time.Duration(1<<63 - 1))
		} else {
			time.Sleep(60 * time.Second)
			records <- metrics.RecordEntry{
				RecordType: metrics.RecordEntryTypeClean,
			}
		}
	}
}

func main() {
	cmd := app.Command(action)
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(cmd.ErrWriter, err)
		os.Exit(1)
	}
}
