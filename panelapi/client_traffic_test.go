package panelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientMatchesShadowsocksInbound(t *testing.T) {
	client := NewClient("https://panel.example", "tok", "31", "shadowsocks")
	if !client.matchesInbound("ss-in") {
		t.Fatal("expected shadowsocks node to match ss-in")
	}
}

func TestPushTrafficIncludesShadowsocksInbound(t *testing.T) {
	var (
		requests int
		payload  PushRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/push" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("node_type") != "shadowsocks" {
			t.Fatalf("node_type = %s", r.URL.Query().Get("node_type"))
		}
		requests++
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "tok", "31", "shadowsocks")
	err := client.PushTraffic(context.Background(), map[string]map[string][2]int64{
		"ss-in": {
			"7":      {10, 20},
			"name-7": {99, 99},
		},
	})
	if err != nil {
		t.Fatalf("PushTraffic: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if got, ok := payload[7]; !ok || len(got) != 2 || got[0] != 10 || got[1] != 20 {
		t.Fatalf("payload[7] = %#v, want [10 20]", got)
	}
}
