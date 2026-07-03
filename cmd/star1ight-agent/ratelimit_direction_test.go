package main

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"star1ight-agent/counter"
	"star1ight-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
)

func TestUserRateLimitAppliesBeforeDownloadWrite(t *testing.T) {
	if !userRateLimitBuildEnabled {
		t.Skip("user rate limiting is compile-time disabled")
	}
	users := NewUserManager(0)
	if err := users.ApplyBox(map[string]adapter.Inbound{}, []panelapi.User{{ID: 7, UUID: "uuid-7", SpeedLimit: 1}}); err != nil {
		t.Fatal(err)
	}
	h := &Hook{users: users}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	wrapped := h.RoutedConnection(context.Background(), server, adapter.InboundContext{
		Inbound: "vless-tcp",
		User:    "uuid-7",
		Source:  M.ParseSocksaddr("127.0.0.1:12345"),
	}, nil, nil)
	if _, ok := wrapped.(*counter.RateLimitedConn); !ok {
		t.Fatalf("connection was not rate limited: %T", wrapped)
	}

	done := make(chan error, 1)
	go func() {
		_, err := io.CopyN(io.Discard, client, 2*1000*1000)
		done <- err
	}()

	start := time.Now()
	if _, err := wrapped.Write(make([]byte, 2*1000*1000)); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("download write bypassed user rate limit: elapsed=%v", elapsed)
	}
}
