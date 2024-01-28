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
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	pq                 *UpdatablePriorityQueue
	totalClients       *prometheus.CounterVec
	totalClientsClosed *prometheus.CounterVec
	totalBytes         *prometheus.CounterVec
	totalSeconds       *prometheus.CounterVec
	clientIP           *prometheus.CounterVec
	clientSeconds      *prometheus.CounterVec
)

func InitPrometheus(prometheusHost, prometheusPort, prometheusEntry string) {
	pq = NewUpdatablePriorityQueue()
	totalClients = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endlessh_client_open_count_total",
			Help: "Total number of clients that tried to connect to this host.",
		}, []string{"local_port"},
	)
	totalClientsClosed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endlessh_client_closed_count_total",
			Help: "Total number of clients that stopped connecting to this host.",
		}, []string{"local_port"},
	)
	totalBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endlessh_sent_bytes_total",
			Help: "Total bytes sent to clients that tried to connect to this host.",
		}, []string{"local_port"},
	)
	totalSeconds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endlessh_trapped_time_seconds_total",
			Help: "Total seconds clients spent on endlessh.",
		}, []string{"local_port"},
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
		if err := http.ListenAndServe(prometheusHost+":"+prometheusPort, nil); err != nil {
			glog.Errorf("Error starting Prometheus at port %v:%v: %v", prometheusHost, prometheusPort, err)
			os.Exit(1)
		}
	}()
}

const (
	RecordEntryTypeStart = iota
	RecordEntryTypeSend  = iota
	RecordEntryTypeStop  = iota
	RecordEntryTypeClean = iota
)

type RecordEntry struct {
	RecordType        int
	IpAddr            string
	LocalPort         string
	MillisecondsSpent int64
	BytesSent         int
}

func StartRecording(maxClients int64, prometheusEnabled bool, prometheusCleanUnseenSeconds int, geoOption geoip.GeoOption) chan RecordEntry {
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
				totalClients.With(prometheus.Labels{"local_port": r.LocalPort}).Inc()
				pq.Update(r.IpAddr, time.Now())
			case RecordEntryTypeSend:
				secondsSpent := float64(r.MillisecondsSpent) / 1000
				clientSeconds.With(prometheus.Labels{
					"ip":         r.IpAddr,
					"local_port": r.LocalPort}).Add(secondsSpent)
				totalSeconds.With(prometheus.Labels{"local_port": r.LocalPort}).Add(secondsSpent)
				totalBytes.With(prometheus.Labels{"local_port": r.LocalPort}).Add(float64(r.BytesSent))
				pq.Update(r.IpAddr, time.Now())
			case RecordEntryTypeStop:
				secondsSpent := float64(r.MillisecondsSpent) / 1000
				clientSeconds.With(prometheus.Labels{
					"ip":         r.IpAddr,
					"local_port": r.LocalPort}).Add(secondsSpent)
				totalSeconds.With(prometheus.Labels{"local_port": r.LocalPort}).Add(secondsSpent)
				totalClientsClosed.With(prometheus.Labels{"local_port": r.LocalPort}).Inc()
				pq.Update(r.IpAddr, time.Now())
			case RecordEntryTypeClean:
				top := pq.Peek()
				deadline := time.Now().Add(-time.Second * time.Duration(prometheusCleanUnseenSeconds))
				for top != nil && top.Value.Before(deadline) {
					clientIP.DeletePartialMatch(prometheus.Labels{"ip": top.Key})
					clientSeconds.DeletePartialMatch(prometheus.Labels{"ip": top.Key})
					pq.Pop()
					top = pq.Peek()
				}
			}
		}
	}()
	return records
}
