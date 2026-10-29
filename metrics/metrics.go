// Copyright (C) 2024-2026 Shizun Ge
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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
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

func InitPrometheus(prometheusHost, prometheusPort, prometheusEntry string) error {
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
	scrapes := make(chan struct{}, 4)
	limitedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case scrapes <- struct{}{}:
			defer func() { <-scrapes }()
			handler.ServeHTTP(w, r)
		default:
			http.Error(w, "too many concurrent scrapes", http.StatusServiceUnavailable)
		}
	})
	mux := http.NewServeMux()
	mux.Handle("/"+prometheusEntry, limitedHandler)
	var listener net.Listener
	var err error
	if strings.HasPrefix(prometheusHost, "unix:") {
		socketPath := strings.TrimPrefix(prometheusHost, "unix:")
		if info, statErr := os.Lstat(socketPath); statErr == nil {
			if info.Mode()&os.ModeSocket == 0 {
				return fmt.Errorf("Prometheus socket path %q exists and is not a socket", socketPath)
			}
			conn, dialErr := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
			if dialErr == nil {
				conn.Close()
				return fmt.Errorf("Prometheus socket %q is already in use", socketPath)
			}
			if !errors.Is(dialErr, syscall.ECONNREFUSED) {
				return fmt.Errorf("check Prometheus socket %q: %w", socketPath, dialErr)
			}
			if err := os.Remove(socketPath); err != nil {
				return fmt.Errorf("remove stale Prometheus socket: %w", err)
			}
		} else if !os.IsNotExist(statErr) {
			return fmt.Errorf("inspect Prometheus socket path: %w", statErr)
		}
		listener, err = net.Listen("unix", socketPath)
		if err != nil {
			return fmt.Errorf("listen on Prometheus socket %q: %w", socketPath, err)
		}
		glog.Infof("Starting Prometheus on Unix socket %v, entry point is /%v", socketPath, prometheusEntry)
	} else {
		addr := net.JoinHostPort(prometheusHost, prometheusPort)
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("listen on Prometheus address %q: %w", addr, err)
		}
		glog.Infof("Starting Prometheus on IP port %v, entry point is /%v", listener.Addr(), prometheusEntry)
	}
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			glog.Errorf("Error serving Prometheus: %v", err)
			os.Exit(1)
		}
	}()
	return nil
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

func StartRecording(maxClients int64, prometheusEnabled bool, prometheusCleanUnseenSeconds int, maxTrackedIPs int, geoOption geoip.GeoOption) chan RecordEntry {
	records, _ := startRecording(maxClients, prometheusEnabled, prometheusCleanUnseenSeconds, maxTrackedIPs, geoOption)
	return records
}

func startRecording(maxClients int64, prometheusEnabled bool, prometheusCleanUnseenSeconds int, maxTrackedIPs int, geoOption geoip.GeoOption) (chan RecordEntry, <-chan struct{}) {
	records := make(chan RecordEntry, maxClients)
	recordingDone := make(chan struct{})
	go func() {
		defer close(recordingDone)
		type geoLabels struct{ geohash, country, location string }
		type geoState struct {
			labels  geoLabels
			pending bool
			counts  map[string]float64
		}
		type geoJob struct {
			ip    string
			state *geoState
		}
		type geoResult struct {
			job    geoJob
			labels geoLabels
			err    error
		}
		geoCache := make(map[string]*geoState)
		jobs := make(chan geoJob, 128)
		results := make(chan geoResult, 128)
		done := make(chan struct{})
		defer close(done)
		defer close(jobs)
		if prometheusEnabled && geoOption.GeoipSupplier != "off" {
			for i := 0; i < 4; i++ {
				go func() {
					for job := range jobs {
						geohash, country, location, err := geoip.GeohashAndLocation(job.ip, geoOption)
						result := geoResult{job: job, labels: geoLabels{geohash, country, location}, err: err}
						select {
						case results <- result:
						case <-done:
							return
						}
					}
				}()
			}
		}
		resolve := func(ip string, state *geoState, labels geoLabels) {
			state.labels = labels
			state.pending = false
			for port, count := range state.counts {
				clientIP.With(prometheus.Labels{
					"ip": ip, "local_port": port,
					"geohash": labels.geohash, "country": labels.country, "location": labels.location,
				}).Add(count)
			}
			state.counts = nil
		}
		removeIP := func(ip string) {
			clientIP.DeletePartialMatch(prometheus.Labels{"ip": ip})
			clientSeconds.DeletePartialMatch(prometheus.Labels{"ip": ip})
			delete(geoCache, ip)
		}
		touchIP := func(ip string) {
			if !pq.Contains(ip) && pq.Len() >= maxTrackedIPs {
				oldest := pq.Pop()
				removeIP(oldest.Key)
			}
			pq.Update(ip, time.Now())
		}
		for {
			select {
			case result := <-results:
				if geoCache[result.job.ip] != result.job.state {
					continue
				}
				if result.err != nil {
					glog.Warningf("Failed to obtain the geohash of %v: %v.", result.job.ip, result.err)
				}
				resolve(result.job.ip, result.job.state, result.labels)
			case r, more := <-records:
				if !more {
					return
				}
				if !prometheusEnabled {
					continue
				}
				switch r.RecordType {
				case RecordEntryTypeStart:
					touchIP(r.IpAddr)
					state := geoCache[r.IpAddr]
					if state == nil {
						state = &geoState{pending: true, counts: map[string]float64{r.LocalPort: 1}}
						geoCache[r.IpAddr] = state
						if geoOption.GeoipSupplier == "off" {
							resolve(r.IpAddr, state, geoLabels{"s000", "Geohash off", "Geohash off"})
						} else {
							select {
							case jobs <- geoJob{ip: r.IpAddr, state: state}:
							default:
								resolve(r.IpAddr, state, geoLabels{"s000", "Unknown", "Unknown"})
							}
						}
					} else if state.pending {
						state.counts[r.LocalPort]++
					} else {
						clientIP.With(prometheus.Labels{
							"ip": r.IpAddr, "local_port": r.LocalPort,
							"geohash": state.labels.geohash, "country": state.labels.country, "location": state.labels.location,
						}).Inc()
					}
					totalClients.With(prometheus.Labels{"local_port": r.LocalPort}).Inc()
				case RecordEntryTypeSend:
					touchIP(r.IpAddr)
					secondsSpent := float64(r.MillisecondsSpent) / 1000
					clientSeconds.With(prometheus.Labels{
						"ip":         r.IpAddr,
						"local_port": r.LocalPort}).Add(secondsSpent)
					totalSeconds.With(prometheus.Labels{"local_port": r.LocalPort}).Add(secondsSpent)
					totalBytes.With(prometheus.Labels{"local_port": r.LocalPort}).Add(float64(r.BytesSent))
				case RecordEntryTypeStop:
					touchIP(r.IpAddr)
					secondsSpent := float64(r.MillisecondsSpent) / 1000
					clientSeconds.With(prometheus.Labels{
						"ip":         r.IpAddr,
						"local_port": r.LocalPort}).Add(secondsSpent)
					totalSeconds.With(prometheus.Labels{"local_port": r.LocalPort}).Add(secondsSpent)
					totalClientsClosed.With(prometheus.Labels{"local_port": r.LocalPort}).Inc()
				case RecordEntryTypeClean:
					top := pq.Peek()
					deadline := time.Now().Add(-time.Second * time.Duration(prometheusCleanUnseenSeconds))
					for top != nil && top.Value.Before(deadline) {
						pq.Pop()
						removeIP(top.Key)
						top = pq.Peek()
					}
				}
			}
		}
	}()
	return records, recordingDone
}
