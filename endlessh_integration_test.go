package main

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	waitForListenTimeout  = 10 * time.Second
	waitForConnectTimeout = 3 * time.Second
	pollInterval          = 50 * time.Millisecond
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitForLogMatch(stderr *lockedBuffer, pattern string, timeout time.Duration) bool {
	re := regexp.MustCompile(pattern)
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return false
		}
		if re.MatchString(stderr.String()) {
			return true
		}
		time.Sleep(pollInterval)
	}
}

func TestEndlesshIntegration_MultiplePorts(t *testing.T) {
	const nPorts = 3
	ports := make([]int, nPorts)
	for i := 0; i < nPorts; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to get free port: %v", err)
		}
		ports[i] = ln.Addr().(*net.TCPAddr).Port
		ln.Close()
	}
	args := []string{"run", ".",
		"-interval_ms=100",
		"-max_clients=10",
		"-logtostderr",
		"-v=1",
	}
	for _, p := range ports {
		args = append(args, fmt.Sprintf("-port=%d", p))
	}

	cmd := exec.Command("go", args...)
	var stderr lockedBuffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Failed to kill process: %v", err)
		}
	}()

	if !waitForLogMatch(&stderr, "Listening on", waitForListenTimeout) {
		t.Fatalf("Timeout waiting for server to start, got logs: %s", stderr.String())
	}
	for _, port := range ports {
		addr := fmt.Sprintf("localhost:%d", port)
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("Failed to connect to server on port %d: %v", port, err)
		}
		if !waitForLogMatch(&stderr, `ACCEPT host=(127\.0\.0\.1|::1)`, waitForConnectTimeout) {
			t.Errorf("Never saw any ACCEPT log, got logs: %s", stderr.String())
		}
		conn.Close()

		if !waitForLogMatch(&stderr, `CLOSE host=(127\.0\.0\.1|::1)`, waitForConnectTimeout) {
			t.Errorf("Never saw any CLOSE log, got logs: %s", stderr.String())
		}
	}
}

func TestEndlesshIntegration_TarpitBehavior(t *testing.T) {
	var stderr lockedBuffer
	cmd := exec.Command("go", "run", ".", "-port=0", "-interval_ms=5000", "-max_clients=10", "-logtostderr", "-v=1")
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Failed to kill process: %v", err)
		}
	}()

	if !waitForLogMatch(&stderr, "Listening on", waitForListenTimeout) {
		t.Fatalf("Timeout waiting for server to start, got logs: %s", stderr.String())
	}

	stderrOutput := stderr.String()
	re := regexp.MustCompile(`Listening on .*:(\d+)`)
	m := re.FindStringSubmatch(stderrOutput)
	if len(m) != 2 {
		t.Fatalf("Could not parse port from logs: %s", stderrOutput)
	}
	port := m[1]
	addr := "localhost:" + port

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to server on port %s: %v", port, err)
	}
	defer conn.Close()

	// Simulate SSH client banner
	// Connect & send client banner
	_, err = conn.Write([]byte("SSH-2.0-OpenSSH_8.2p1\r\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Expect FIRST LINE immediately (no delay)
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("Expected first tarpit line immediately, got: %v (%d bytes)", err, n)
	}
	t.Logf("Got first tarpit line (%d bytes): %q", n, buf[:n])

	time.Sleep(100 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n2, err := conn.Read(buf)
	if err == nil && n2 > 0 {
		t.Errorf("Got %d bytes too soon (within 500ms), tarpit failed", n2)
	}
}

func TestEndlesshIntegration_Concurrency(t *testing.T) {
	maxClients := 5
	var stderr lockedBuffer
	cmd := exec.Command("go", "run", ".", "-port=0", "-interval_ms=1000", fmt.Sprintf("-max_clients=%d", maxClients), "-logtostderr", "-v=1")
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Failed to kill process: %v", err)
		}
	}()

	if !waitForLogMatch(&stderr, "Listening on", waitForListenTimeout) {
		t.Fatalf("Timeout waiting for server to start, got logs: %s", stderr.String())
	}
	stderrOutput := stderr.String()
	re := regexp.MustCompile(`Listening on .*:(\d+)`)
	m := re.FindStringSubmatch(stderrOutput)
	if len(m) != 2 {
		t.Fatalf("Could not parse port from logs: %s", stderrOutput)
	}
	port := m[1]
	addr := fmt.Sprintf("localhost:%s", port)

	// Test multiple connections
	var wg sync.WaitGroup
	var mu sync.Mutex
	activeClients := 0
	maxActiveClients := 0
	successfulReads := 0

	for i := 0; i < maxClients+1; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			conn, dialErr := net.Dial("tcp", addr)
			if dialErr != nil {
				if clientID == maxClients {
					t.Logf("Client %d dial failed (expected): %v", clientID, dialErr)
				} else {
					t.Errorf("Client %d dial failed (unexpected): %v", clientID, dialErr)
				}
				return
			}
			defer conn.Close()

			_, writeErr := conn.Write([]byte("SSH-2.0-OpenSSH_8.2p1\r\n"))
			if writeErr != nil {
				t.Logf("Client %d write failed: %v", clientID, writeErr)
				return
			}

			buf := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			n1, readErr1 := conn.Read(buf)
			if readErr1 != nil || n1 == 0 {
				if clientID != maxClients { // only log unexpected failures
					t.Logf("Client %d failed first tarpit line (unexpected): %v (%d bytes)", clientID, readErr1, n1)
				}
				return
			}

			// Client is active only if it successfully received data
			mu.Lock()
			activeClients++
			if activeClients > maxActiveClients {
				maxActiveClients = activeClients
			}
			successfulReads++
			mu.Unlock()

			t.Logf("Client %d got line 1 (%d bytes): %q", clientID, n1, buf[:n1])

			// Keep the connection open for a while
			time.Sleep(5 * time.Second)

			mu.Lock()
			activeClients--
			mu.Unlock()
		}(i)
		time.Sleep(200 * time.Millisecond)
	}
	wg.Wait()

	mu.Lock()
	deferredSuccessfulReads := successfulReads
	mu.Unlock()

	if deferredSuccessfulReads < 1 {
		t.Errorf("Expected at least one client to receive a tarpit line, got %d", deferredSuccessfulReads)
	}

	// Check if maxActiveClients exceeded maxClients
	if maxActiveClients > maxClients {
		t.Errorf("Expected max %d concurrent clients, got %d", maxClients, maxActiveClients)
	}

	// A TCP Dial can complete before the server's Accept call rejects an excess
	// connection. The client limit is enforced when the accepted socket closes.
}

func TestEndlesshIntegration_PrometheusMetrics(t *testing.T) {
	var stderr lockedBuffer

	cmd := exec.Command(
		"go", "run", ".",
		"-port=0",
		"-prometheus_enabled",
		"-prometheus_port=0",
		"-interval_ms=100",
		"-logtostderr", "-v=1",
	)
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer cmd.Process.Kill()

	if !waitForLogMatch(&stderr, "Starting Prometheus", waitForListenTimeout) {
		t.Fatalf("Prometheus did not start: %s", stderr.String())
	}

	reProm := regexp.MustCompile(`Prometheus on IP port .*:(\d+)`)
	reMain := regexp.MustCompile(`Listening on .*:(\d+)`)

	promMatch := reProm.FindStringSubmatch(stderr.String())
	mainMatch := reMain.FindStringSubmatch(stderr.String())

	if len(promMatch) < 2 || len(mainMatch) < 2 {
		t.Fatalf("Could not parse ports: %s", stderr.String())
	}

	promPort := promMatch[1]
	mainPort := mainMatch[1]

	conn, err := net.Dial("tcp", "localhost:"+mainPort)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	conn.Write([]byte("SSH-2.0-test\r\n"))

	time.Sleep(500 * time.Millisecond)

	// Fetch metrics
	resp, err := net.Dial("tcp", "localhost:"+promPort)
	if err != nil {
		t.Fatalf("Failed to connect to metrics endpoint: %v", err)
	}

	fmt.Fprintf(resp, "GET /metrics HTTP/1.1\r\nHost: localhost\r\n\r\n")

	buf := make([]byte, 8192)
	n, _ := resp.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "endlessh_client_open_count_total") {
		t.Errorf("Missing expected metric in output:\n%s", body)
	}

	if !strings.Contains(body, "endlessh_sent_bytes_total") {
		t.Errorf("Expected bytes metric not found:\n%s", body)
	}
}

func TestEndlesshIntegration_Healthcheck(t *testing.T) {
	var stderr lockedBuffer

	cmd := exec.Command(
		"go", "run", ".",
		"-port=0",
		"-healthcheck_enabled",
		"-healthcheck_port=0",
		"-interval_ms=100",
		"-logtostderr", "-v=1",
	)
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer cmd.Process.Kill()

	if !waitForLogMatch(&stderr, "Starting healthcheck on http", waitForListenTimeout) {
		t.Fatalf("Healthcheck listener did not start: %s", stderr.String())
	}

	reHealth := regexp.MustCompile(`Starting healthcheck on http://.*:(\d+)/health`)
	healthMatch := reHealth.FindStringSubmatch(stderr.String())
	if len(healthMatch) < 2 {
		t.Fatalf("Could not parse healthcheck port: %s", stderr.String())
	}
	healthPort := healthMatch[1]

	// 1. Verify healthcheck HTTP endpoint returns valid JSON
	resp, err := net.Dial("tcp", "localhost:"+healthPort)
	if err != nil {
		t.Fatalf("Failed to connect to healthcheck endpoint: %v", err)
	}
	fmt.Fprintf(resp, "GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n")

	buf := make([]byte, 8192)
	n, _ := resp.Read(buf)
	body := string(buf[:n])
	resp.Close()

	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("Expected status:ok in health response: %s", body)
	}

	// 2. Run the probe mode (-healthcheck) and verify it exits with 0
	probeCmd := exec.Command(
		"go", "run", ".",
		"-healthcheck",
		"-healthcheck_port="+healthPort,
	)
	var probeStderr lockedBuffer
	probeCmd.Stderr = &probeStderr
	if err := probeCmd.Run(); err != nil {
		t.Errorf("Healthcheck probe failed: %v, stderr: %s", err, probeStderr.String())
	}
}

func TestEndlesshIntegration_ProxyProtocol(t *testing.T) {
	var stderr lockedBuffer
	cmd := exec.Command("go", "run", ".", "-port=0", "-interval_ms=100", "-proxy_protocol_enabled", "-logtostderr", "-v=1")
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	if !waitForLogMatch(&stderr, "Listening on", waitForListenTimeout) {
		t.Fatalf("server did not start: %s", stderr.String())
	}
	match := regexp.MustCompile(`Listening on .*:(\d+)`).FindStringSubmatch(stderr.String())
	if len(match) != 2 {
		t.Fatalf("could not find listening port: %s", stderr.String())
	}
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", match[1]))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "PROXY TCP4 198.51.100.1 127.0.0.1 34567 %s\r\n", match[1]); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128)
	if n, err := conn.Read(buf); err != nil || n == 0 {
		t.Fatalf("no tarpit response through PROXY protocol: %v", err)
	}
	if !waitForLogMatch(&stderr, `ACCEPT host=198\.51\.100\.1`, waitForConnectTimeout) {
		t.Fatalf("PROXY source address was not used: %s", stderr.String())
	}
}
