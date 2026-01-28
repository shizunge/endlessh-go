package main

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func freePort(t *testing.T, port string) {
	cmd := exec.Command("sudo", "lsof", "-i", ":"+port)
	output, err := cmd.Output()
	if err != nil {
		t.Logf("No process found using port %s, or error: %v", port, err)
		return
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "LISTEN") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				pid := fields[1]
				killCmd := exec.Command("sudo", "kill", "-9", pid)
				err := killCmd.Run()
				if err != nil {
					t.Logf("Failed to kill process %s: %v", pid, err)
				} else {
					t.Logf("Freed port %s by killing process %s", port, pid)
				}
			}
		}
	}
}

func TestEndlesshIntegration_MultiplePorts(t *testing.T) {
	ports := []string{"2223", "2224", "2225"}
	for _, port := range ports {
		freePort(t, port)

		cmd := exec.Command("go", "run", "main.go", "-port="+port, "-interval_ms=100", "-max_clients=10", "-logtostderr", "-v=1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		err := cmd.Start()
		if err != nil {
			t.Fatalf("Failed to start server on port %s: %v", port, err)
		}

		time.Sleep(1 * time.Second)

		stderrOutput := stderr.String()
		if !strings.Contains(stderrOutput, "Listening on") {
			t.Errorf("Expected 'Listening on' in logs for port %s, got: %s", port, stderrOutput)
		}

		conn, err := net.Dial("tcp", "localhost:"+port)
		if err != nil {
			t.Fatalf("Failed to connect to server on port %s: %v", port, err)
		}
		conn.Close()

		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Failed to kill process for port %s: %v", port, err)
		}
	}
}

func TestEndlesshIntegration_TarpitBehavior(t *testing.T) {
	freePort(t, "2224")

	cmd := exec.Command("go", "run", "main.go", "-port=2224", "-interval_ms=5000", "-max_clients=10", "-logtostderr", "-v=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Failed to kill process: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	stderrOutput := stderr.String()
	if !strings.Contains(stderrOutput, "Listening on") {
		t.Fatalf("Expected 'Listening on' in logs, got: %s", stderrOutput)
	}

	// Connect and simulate an SSH client
	conn, err := net.Dial("tcp", "localhost:2224")
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
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
	t.Logf("Got first tarpit line (%d bytes): %q", n, buf[:n]) // Logs your random garbage ✓

	// Wait LESS THAN interval (e.g. 500ms < 3000ms interval)
	time.Sleep(500 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n2, err := conn.Read(buf)
	if err == nil && n2 > 0 {
		t.Errorf("Got %d bytes too soon (within 500ms), tarpit failed", n2)
	}

}

func TestEndlesshIntegration_Concurrency(t *testing.T) {
	freePort(t, "2225")

	maxClients := 5
	cmd := exec.Command("go", "run", "main.go", "-port=2225", "-interval_ms=1000", fmt.Sprintf("-max_clients=%d", maxClients), "-logtostderr", "-v=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Failed to kill process: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	stderrOutput := stderr.String()
	if !strings.Contains(stderrOutput, "Listening on") {
		t.Fatalf("Expected 'Listening on' in logs, got: %s", stderrOutput)
	}

	// Test multiple connections
	var wg sync.WaitGroup
	var mu sync.Mutex
	activeClients := 0
	maxActiveClients := 0
	sixthClientFailed := false

	for i := 0; i < maxClients+1; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			conn, dialErr := net.Dial("tcp", "localhost:2225")
			if dialErr != nil {
				t.Logf("Client %d failed to connect: %v", clientID, dialErr)
				return
			}
			defer conn.Close()

			_, writeErr := conn.Write([]byte("SSH-2.0-OpenSSH_8.2p1\r\n"))
			if writeErr != nil {
				t.Logf("Client %d write failed: %v", clientID, writeErr)
				return
			}

			buf := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n1, readErr1 := conn.Read(buf)
			if readErr1 != nil {
				if clientID == maxClients { // 6th client (0-indexed: 5)
					mu.Lock()
					sixthClientFailed = true
					mu.Unlock()
					t.Logf("Client %d failed first tarpit line (expected): %v (%d bytes)", clientID, readErr1, n1)
				} else {
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

	// Check if maxActiveClients exceeded maxClients
	if maxActiveClients > maxClients {
		t.Errorf("Expected max %d concurrent clients, got %d", maxClients, maxActiveClients)
	}

	// Check if the 6th client failed
	mu.Lock()
	if !sixthClientFailed {
		t.Errorf("Expected 6th client to fail, but it succeeded")
	}
	mu.Unlock()
}
