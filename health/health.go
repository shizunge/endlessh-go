// Copyright (C) 2026 Paolo Asperti
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

package health

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"
)

const (
	DefaultHost    = "127.0.0.1"
	DefaultPort    = "51000"
	DefaultPath    = "/health"
	DefaultTimeout = 3 * time.Second
)

type response struct {
	Status string  `json:"status"` // always "ok"
	Uptime float64 `json:"uptime"` // in seconds
}

var startTime time.Time

func StartListener(host, port string) (string, error) {
	mux := http.NewServeMux()
	mux.HandleFunc(DefaultPath, handleHealth)
	addr := net.JoinHostPort(host, port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	startTime = time.Now()
	actualPort := l.Addr().(*net.TCPAddr).Port
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	actualAddr := net.JoinHostPort(host, fmt.Sprint(actualPort))
	glog.Infof("Starting healthcheck on http://%v%v", actualAddr, DefaultPath)
	go func() {
		if err := server.Serve(l); err != nil && err != http.ErrServerClosed {
			glog.Errorf("Error serving healthcheck on %v: %v", actualAddr, err)
			os.Exit(1)
		}
	}()
	return fmt.Sprint(actualPort), nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{
		Status: "ok",
		Uptime: time.Since(startTime).Seconds(),
	})
}

func Probe(host, port string) bool {
	addr := net.JoinHostPort(host, port)
	url := "http://" + addr + DefaultPath
	client := &http.Client{Timeout: DefaultTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var body response
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}
	return body.Status == "ok"
}

// SetupHealthcheck probes or starts the healthcheck listener as configured.
func SetupHealthcheck(healthcheck bool, healthcheckEnabled bool, connType string, healthcheckHost, healthcheckPort *string) {
	if healthcheck {
		if !Probe(*healthcheckHost, *healthcheckPort) {
			os.Exit(1)
		}
		os.Exit(0)
	}

	if healthcheckEnabled {
		if connType == "tcp6" && *healthcheckHost == "0.0.0.0" {
			*healthcheckHost = "::"
		}
		if *healthcheckPort == "" {
			*healthcheckPort = "0"
		}
		actualPort, err := StartListener(*healthcheckHost, *healthcheckPort)
		if err != nil {
			glog.Fatalf("Failed to start healthcheck listener: %v", err)
		}
		*healthcheckPort = actualPort
	}
}
