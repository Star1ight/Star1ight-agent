package panelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPushAliveIncludesOnlyMatchingInboundAndNumericUsers(t *testing.T) {
	var (
		requests int
		payload  map[string][]string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/alive" {
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
	err := client.PushAlive(context.Background(), map[string]map[string][]string{
		"vless-in": {
			"1": {"198.51.100.10"},
		},
		"ss-in": {
			"7":      {"203.0.113.7"},
			"8":      {},
			"name-7": {"198.51.100.99"},
		},
	})
	if err != nil {
		t.Fatalf("PushAlive: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if got := payload["7"]; len(got) != 1 || got[0] != "203.0.113.7" {
		t.Fatalf("payload[7] = %#v, want [203.0.113.7]", got)
	}
	if got := payload["8"]; got == nil || len(got) != 0 {
		t.Fatalf("payload[8] = %#v, want empty tombstone list", got)
	}
	if _, ok := payload["1"]; ok {
		t.Fatalf("payload unexpectedly included mismatched inbound user: %#v", payload)
	}
	if _, ok := payload["name-7"]; ok {
		t.Fatalf("payload unexpectedly included non-numeric user: %#v", payload)
	}
}
