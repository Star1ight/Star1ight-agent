package main

import (
	"context"
	"io"
	"net"
	"reflect"
	"testing"

	"star1ight-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
)

func TestHookAliveDeltaTracksDevicesAndClearsClosedSessions(t *testing.T) {
	um := NewUserManager(0)
	if err := um.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", Password: "pw-7", Name: "name-7"}}); err != nil {
		t.Fatal(err)
	}
	classifier, err := ParseSourceBuckets([]string{
		"nbix=114.111.176.34/32",
		"cnix=103.96.140.122/32",
	})
	if err != nil {
		t.Fatalf("ParseSourceBuckets: %v", err)
	}
	h := &Hook{
		users:          um,
		sessionTracker: NewSessionTracker(classifier),
	}

	client, server := net.Pipe()
	defer client.Close()

	wrapped := h.RoutedConnection(context.Background(), server, adapter.InboundContext{
		Inbound: "ss-in",
		User:    "uuid-7",
		Source:  M.ParseSocksaddr("114.111.176.34:2608"),
	}, nil, nil)

	payload := make([]byte, 64*1024)
	go func() {
		_, _ = client.Write(payload)
	}()
	readBuf := make([]byte, len(payload))
	if n, err := io.ReadFull(wrapped, readBuf); err != nil || n != len(payload) {
		t.Fatalf("ReadFull = %d, %v; want %d, nil", n, err, len(payload))
	}

	alive := h.AliveDelta()
	wantAlive := map[string]map[string][]string{
		"ss-in": {
			"7": {"114.111.176.34"},
		},
	}
	if !reflect.DeepEqual(alive, wantAlive) {
		t.Fatalf("AliveDelta = %#v, want %#v", alive, wantAlive)
	}

	sources := h.SourceSnapshot()
	wantSources := map[string]map[string]map[string]int{
		"ss-in": {
			"7": {"nbix": 1},
		},
	}
	if !reflect.DeepEqual(sources, wantSources) {
		t.Fatalf("SourceSnapshot = %#v, want %#v", sources, wantSources)
	}

	h.CommitAlive(alive)
	if next := h.AliveDelta(); len(next) != 0 {
		t.Fatalf("AliveDelta after commit = %#v, want empty", next)
	}

	if err := wrapped.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	tombstone := h.AliveDelta()
	wantTombstone := map[string]map[string][]string{
		"ss-in": {
			"7": {},
		},
	}
	if !reflect.DeepEqual(tombstone, wantTombstone) {
		t.Fatalf("AliveDelta after close = %#v, want %#v", tombstone, wantTombstone)
	}
}

func TestHookAliveDeltaIgnoresZeroByteConnections(t *testing.T) {
	um := NewUserManager(0)
	if err := um.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", Password: "pw-7", Name: "name-7"}}); err != nil {
		t.Fatal(err)
	}
	h := &Hook{
		users:          um,
		sessionTracker: NewSessionTracker(nil),
	}

	client, server := net.Pipe()
	defer client.Close()

	wrapped := h.RoutedConnection(context.Background(), server, adapter.InboundContext{
		Inbound: "vless-in",
		User:    "uuid-7",
		Source:  M.ParseSocksaddr("183.222.236.42:2095"),
	}, nil, nil)

	if alive := h.AliveDelta(); len(alive) != 0 {
		t.Fatalf("AliveDelta before traffic = %#v, want empty", alive)
	}
	if sources := h.SourceSnapshot(); len(sources) != 0 {
		t.Fatalf("SourceSnapshot before traffic = %#v, want empty", sources)
	}

	if err := wrapped.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if alive := h.AliveDelta(); len(alive) != 0 {
		t.Fatalf("AliveDelta after zero-byte close = %#v, want empty", alive)
	}
}

func TestHookAliveDeltaIgnoresShortFailedHandshakeTraffic(t *testing.T) {
	um := NewUserManager(0)
	if err := um.ApplyBox(nil, []panelapi.User{{ID: 5, UUID: "uuid-5", Password: "pw-5", Name: "name-5"}}); err != nil {
		t.Fatal(err)
	}
	h := &Hook{
		users:          um,
		sessionTracker: NewSessionTracker(nil),
	}

	client, server := net.Pipe()
	defer client.Close()

	wrapped := h.RoutedConnection(context.Background(), server, adapter.InboundContext{
		Inbound: "vless-in",
		User:    "uuid-5",
		Source:  M.ParseSocksaddr("183.222.236.42:2095"),
	}, nil, nil)

	payload := make([]byte, 1024)
	go func() {
		_, _ = client.Write(payload)
	}()
	readBuf := make([]byte, len(payload))
	if n, err := io.ReadFull(wrapped, readBuf); err != nil || n != len(payload) {
		t.Fatalf("ReadFull = %d, %v; want %d, nil", n, err, len(payload))
	}

	if alive := h.AliveDelta(); len(alive) != 0 {
		t.Fatalf("AliveDelta after short failed traffic = %#v, want empty", alive)
	}
	if sources := h.SourceSnapshot(); len(sources) != 0 {
		t.Fatalf("SourceSnapshot after short failed traffic = %#v, want empty", sources)
	}

	if err := wrapped.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if alive := h.AliveDelta(); len(alive) != 0 {
		t.Fatalf("AliveDelta after short failed close = %#v, want empty", alive)
	}
}
