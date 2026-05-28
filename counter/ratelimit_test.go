package counter

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestRateLimitedConnWriteIsSmoothed(t *testing.T) {
	limiter := NewRateLimiter(1_000_000) // 8 Mbps, easy to measure quickly.
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	wrapped := NewRateLimitedConn(server, nil, limiter)
	payload := make([]byte, 2_000_000)
	firstRead := make(chan time.Duration, 1)
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		buf := make([]byte, 32*1024)
		total := 0
		first := true
		for total < len(payload) {
			n, err := client.Read(buf)
			if n > 0 {
				if first {
					firstRead <- time.Since(start)
					first = false
				}
				total += n
			}
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	go func() {
		_, _ = wrapped.Write(payload)
	}()

	select {
	case d := <-firstRead:
		if d > 700*time.Millisecond {
			t.Fatalf("reader did not receive early data; limiter appears to wait for the whole write before sending")
		}
	case <-time.After(900 * time.Millisecond):
		t.Fatal("reader timed out waiting for first bytes")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 1700*time.Millisecond || elapsed > 3500*time.Millisecond {
		t.Fatalf("expected smoothed 2MB transfer near 2s, got %s", elapsed)
	}
}

func TestRateLimitedConnReadIsBoundedAndSmoothed(t *testing.T) {
	limiter := NewRateLimiter(1_000_000) // 8 Mbps.
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	wrapped := NewRateLimitedConn(server, limiter, nil)
	payload := make([]byte, 2_000_000)
	go func() {
		defer client.Close()
		_, _ = client.Write(payload)
	}()

	buf := make([]byte, 512*1024)
	start := time.Now()
	n, err := wrapped.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n > maxReadChunk {
		t.Fatalf("read returned %d bytes, want bounded to <= %d", n, maxReadChunk)
	}
	if time.Since(start) > 700*time.Millisecond {
		t.Fatalf("first bounded read took too long: %s", time.Since(start))
	}

	total := n
	for total < len(payload) {
		n, err := wrapped.Read(buf)
		if n > maxReadChunk {
			t.Fatalf("read returned %d bytes, want bounded to <= %d", n, maxReadChunk)
		}
		if n > 0 {
			total += n
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 1700*time.Millisecond || elapsed > 3500*time.Millisecond {
		t.Fatalf("expected smoothed 2MB read near 2s, got %s", elapsed)
	}
}

func TestRateLimiterSharedAcrossMultipleConnections(t *testing.T) {
	limiter := NewRateLimiter(1_000_000) // 8 Mbps total shared by all conns.
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		client, server := net.Pipe()
		wrapped := NewRateLimitedConn(server, nil, limiter)
		payload := make([]byte, 500_000)
		wg.Add(2)
		go func() {
			defer wg.Done()
			defer client.Close()
			_, _ = io.Copy(io.Discard, client)
		}()
		go func() {
			defer wg.Done()
			defer server.Close()
			_, _ = wrapped.Write(payload)
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	if elapsed < 1700*time.Millisecond || elapsed > 4000*time.Millisecond {
		t.Fatalf("expected 2MB across shared limiter near 2s, got %s", elapsed)
	}
}
