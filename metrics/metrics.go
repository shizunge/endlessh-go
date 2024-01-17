// Copyright (C) 2024 Shizun Ge
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

package metrics

import (
	"endlessh-go/geoip"
	"net/http"
	"sync/atomic"

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

func InitPrometheus(prometheusHost, prometheusPort, prometheusEntry string) {
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
		[]string{"ip", "local_port", "geohash", "country", "location"},
	)
	clientSeconds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endlessh_client_trapped_time_seconds",
			Help: "Seconds a client spends on endlessh.",
		},
		[]string{"ip", "local_port"},
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
	RecordEntryTypeStart = iota
	RecordEntryTypeSend  = iota
	RecordEntryTypeStop  = iota
)

type RecordEntry struct {
	RecordType        int
	IpAddr            string
	LocalPort         string
	BytesSent         int
	MillisecondsSpent int64
}

func StartRecording(maxClients int64, prometheusEnabled bool, geoOption geoip.GeoOption) chan RecordEntry {
	records := make(chan RecordEntry, maxClients)
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
			case RecordEntryTypeStart:
				geohash, country, location, err := geoip.GeohashAndLocation(r.IpAddr, geoOption)
				if err != nil {
					glog.Warningf("Failed to obatin the geohash of %v: %v.", r.IpAddr, err)
				}
				clientIP.With(prometheus.Labels{
					"ip":         r.IpAddr,
					"local_port": r.LocalPort,
					"geohash":    geohash,
					"country":    country,
					"location":   location}).Inc()
				atomic.AddInt64(&numTotalClients, 1)
			case RecordEntryTypeSend:
				clientSeconds.With(prometheus.Labels{
					"ip":         r.IpAddr,
					"local_port": r.LocalPort}).Add(float64(r.MillisecondsSpent) / 1000)
				atomic.AddInt64(&numTotalBytes, int64(r.BytesSent))
				atomic.AddInt64(&numTotalMilliseconds, r.MillisecondsSpent)
			case RecordEntryTypeStop:
				atomic.AddInt64(&numTotalClientsClosed, 1)
			}
		}
	}()
	return records
}
