package panelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReportMachineStatusPostsExpectedPayload(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotQuery  string
		gotBody   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "machine-token", "", "")
	client.MachineID = "9"
	client.MachineToken = "machine-token"

	err := client.ReportMachineStatus(context.Background(), MachineStatus{
		CPU: 51.5,
		Mem: UsagePair{Total: 4096, Used: 1024},
		Swap: UsagePair{
			Total: 2048,
			Used:  256,
		},
		Disk: UsagePair{Total: 102400, Used: 51200},
		Net:  &NetSpeed{InSpeed: 123.25, OutSpeed: 456.75},
	})
	if err != nil {
		t.Fatalf("ReportMachineStatus: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/v2/server/machine/status" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotQuery != "" {
		t.Fatalf("query = %q, want empty", gotQuery)
	}
	if gotBody["token"] != "machine-token" {
		t.Fatalf("token = %v", gotBody["token"])
	}
	if gotBody["machine_id"] != "9" {
		t.Fatalf("machine_id = %v", gotBody["machine_id"])
	}
	if gotBody["node_id"] != nil {
		t.Fatalf("node_id unexpectedly present: %v", gotBody["node_id"])
	}

	mem, ok := gotBody["mem"].(map[string]any)
	if !ok || mem["total"] != float64(4096) || mem["used"] != float64(1024) {
		t.Fatalf("mem = %#v", gotBody["mem"])
	}
	swap, ok := gotBody["swap"].(map[string]any)
	if !ok || swap["total"] != float64(2048) || swap["used"] != float64(256) {
		t.Fatalf("swap = %#v", gotBody["swap"])
	}
	disk, ok := gotBody["disk"].(map[string]any)
	if !ok || disk["total"] != float64(102400) || disk["used"] != float64(51200) {
		t.Fatalf("disk = %#v", gotBody["disk"])
	}
	net, ok := gotBody["net"].(map[string]any)
	if !ok || net["in_speed"] != 123.25 || net["out_speed"] != 456.75 {
		t.Fatalf("net = %#v", gotBody["net"])
	}
}

func TestReportMachineStatusOmitsOptionalNetWhenUnavailable(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "machine-token", "", "")
	client.MachineID = "11"
	client.MachineToken = "machine-token"

	err := client.ReportMachineStatus(context.Background(), MachineStatus{
		CPU: 20,
		Mem: UsagePair{Total: 1, Used: 1},
	})
	if err != nil {
		t.Fatalf("ReportMachineStatus: %v", err)
	}
	if _, ok := gotBody["net"]; ok {
		t.Fatalf("net should be omitted, got %#v", gotBody["net"])
	}
	if _, ok := gotBody["swap"]; ok {
		t.Fatalf("swap should be omitted, got %#v", gotBody["swap"])
	}
	if _, ok := gotBody["disk"]; ok {
		t.Fatalf("disk should be omitted, got %#v", gotBody["disk"])
	}
}

func TestReportMachineStatusUsesDedicatedMachineToken(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "panel-token", "", "")
	client.MachineID = "23"
	client.MachineToken = "machine-token"

	err := client.ReportMachineStatus(context.Background(), MachineStatus{
		CPU: 20,
		Mem: UsagePair{Total: 1, Used: 1},
	})
	if err != nil {
		t.Fatalf("ReportMachineStatus: %v", err)
	}
	if gotBody["token"] != "machine-token" {
		t.Fatalf("token = %v, want machine-token", gotBody["token"])
	}
}

func TestReportMachineStatusRequiresMachineToken(t *testing.T) {
	client := NewClient("https://panel.example.com", "panel-token", "", "")
	client.MachineID = "23"

	err := client.ReportMachineStatus(context.Background(), MachineStatus{
		CPU: 20,
		Mem: UsagePair{Total: 1, Used: 1},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
