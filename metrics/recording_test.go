package metrics

import (
	"endlessh-go/geoip"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRecordingEvictsOldestIP(t *testing.T) {
	if err := InitPrometheus("127.0.0.1", "0", "metrics"); err != nil {
		t.Fatal(err)
	}
	records, done := startRecording(8, true, 3600, 2, geoip.GeoOption{GeoipSupplier: "off"})
	defer func() { close(records); <-done }()
	for _, ip := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"} {
		records <- RecordEntry{RecordType: RecordEntryTypeStart, IpAddr: ip, LocalPort: "2222"}
	}
	deadline := time.Now().Add(time.Second)
	for {
		m := &dto.Metric{}
		if err := totalClients.WithLabelValues("2222").Write(m); err != nil {
			t.Fatal(err)
		}
		if m.GetCounter().GetValue() == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for metric records")
		}
		time.Sleep(time.Millisecond)
	}
	ch := make(chan prometheus.Metric, 3)
	clientIP.Collect(ch)
	close(ch)
	seen := make(map[string]bool)
	for metric := range ch {
		m := &dto.Metric{}
		if err := metric.Write(m); err != nil {
			t.Fatal(err)
		}
		for _, label := range m.Label {
			if label.GetName() == "ip" {
				seen[label.GetValue()] = true
			}
		}
	}
	if len(seen) != 2 || seen["192.0.2.1"] || !seen["192.0.2.2"] || !seen["192.0.2.3"] {
		t.Fatalf("tracked IPs after eviction: %v", seen)
	}
}

func TestPrometheusDoesNotRemoveActiveUnixSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/private/tmp", "eg-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "metrics.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := InitPrometheus("unix:"+path, "", "metrics"); err == nil {
		t.Fatal("expected an error for a socket already in use")
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("active socket was removed: %v", err)
	}
	conn.Close()
}

func TestAsyncGeoIPCountsRepeatedConnections(t *testing.T) {
	if err := InitPrometheus("127.0.0.1", "0", "metrics"); err != nil {
		t.Fatal(err)
	}
	records, done := startRecording(8, true, 3600, 2, geoip.GeoOption{GeoipSupplier: "invalid"})
	defer func() { close(records); <-done }()
	for i := 0; i < 3; i++ {
		records <- RecordEntry{RecordType: RecordEntryTypeStart, IpAddr: "192.0.2.1", LocalPort: "2222"}
	}
	deadline := time.Now().Add(time.Second)
	for {
		ch := make(chan prometheus.Metric, 2)
		clientIP.Collect(ch)
		close(ch)
		for metric := range ch {
			m := &dto.Metric{}
			if err := metric.Write(m); err != nil {
				t.Fatal(err)
			}
			if m.GetCounter().GetValue() == 3 {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("per-IP counter did not include all connections")
		}
		time.Sleep(time.Millisecond)
	}
}
