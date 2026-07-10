package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateConfigFromXboardVLESS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/config" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("node_type") != "vless" {
			t.Fatalf("node_type = %s", r.URL.Query().Get("node_type"))
		}
		_, _ = w.Write([]byte(`{"protocol":"vless","listen_ip":"0.0.0.0","server_port":10001,"network":"tcp","flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv","short_id":"sid"},"base_config":{"pull_interval":60,"push_interval":60}}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	gotType, err := generateXboardConfig(context.Background(), xboardGenerateOptions{PanelURL: srv.URL, PanelToken: "tok", PanelNodeID: "21", PanelNodeType: "vless", NodeMode: "vless", Out: out})
	if err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}
	if gotType != "vless" {
		t.Fatalf("node type = %q, want vless", gotType)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("generated config is not json: %v\n%s", err, data)
	}
	opts, err := loadOptions(out)
	if err != nil {
		t.Fatalf("generated config not loadable by sing-box: %v\n%s", err, data)
	}
	if len(opts.Inbounds) != 1 || opts.Inbounds[0].Type != "vless" {
		t.Fatalf("inbounds = %+v", opts.Inbounds)
	}
}

func TestGenerateConfigFromXboardHY2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("node_type") != "hysteria" {
			t.Fatalf("node_type = %s", r.URL.Query().Get("node_type"))
		}
		_, _ = w.Write([]byte(`{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":10001,"version":2,"server_name":"www.apple.com","tls_settings":{"server_name":"www.apple.com","allow_insecure":true},"up_mbps":0,"down_mbps":0,"obfs":"salamander","obfs-password":"obfspass","base_config":{"pull_interval":60,"push_interval":60}}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	gotType, err := generateXboardConfig(context.Background(), xboardGenerateOptions{PanelURL: srv.URL, PanelToken: "tok", PanelNodeID: "22", PanelNodeType: "hysteria", NodeMode: "hy2", Out: out})
	if err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}
	if gotType != "hy2" {
		t.Fatalf("node type = %q, want hy2", gotType)
	}
	opts, err := loadOptions(out)
	if err != nil {
		data, _ := os.ReadFile(out)
		t.Fatalf("generated HY2 config not loadable by sing-box: %v\n%s", err, data)
	}
	if len(opts.Inbounds) != 1 || opts.Inbounds[0].Type != "hysteria2" {
		t.Fatalf("inbounds = %+v", opts.Inbounds)
	}
}

func TestGenerateConfigFromXboardBothVLESSAndHY2(t *testing.T) {
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.URL.Query().Get("node_id")
		nodeType := r.URL.Query().Get("node_type")
		seen[nodeID] = nodeType
		switch nodeID + ":" + nodeType {
		case "21:vless":
			_, _ = w.Write([]byte(`{"protocol":"vless","listen_ip":"0.0.0.0","server_port":10001,"network":"tcp","flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv","short_id":"sid"}}`))
		case "22:hysteria":
			_, _ = w.Write([]byte(`{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":10002,"version":2,"server_name":"www.apple.com","tls_settings":{"server_name":"www.apple.com"},"obfs":"salamander","obfs-password":"obfspass"}`))
		default:
			http.Error(w, `{"message":"unexpected node"}`, http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	mode, err := generateXboardConfig(context.Background(), xboardGenerateOptions{PanelURL: srv.URL, PanelToken: "tok", NodeMode: "both", VLESSNodeID: "21", HY2NodeID: "22", Out: out})
	if err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}
	if mode != "both" {
		t.Fatalf("mode = %q, want both", mode)
	}
	if seen["21"] != "vless" || seen["22"] != "hysteria" {
		t.Fatalf("seen queries = %+v", seen)
	}
	opts, err := loadOptions(out)
	if err != nil {
		data, _ := os.ReadFile(out)
		t.Fatalf("generated dual config not loadable by sing-box: %v\n%s", err, data)
	}
	if len(opts.Inbounds) != 2 || opts.Inbounds[0].Type != "vless" || opts.Inbounds[1].Type != "hysteria2" {
		t.Fatalf("inbounds = %+v", opts.Inbounds)
	}
}

func TestGenerateConfigFromXboardSS2022(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("node_type") != "shadowsocks" {
			t.Fatalf("node_type = %s", r.URL.Query().Get("node_type"))
		}
		_, _ = w.Write([]byte(`{
			"protocol":"shadowsocks",
			"listen_ip":"0.0.0.0",
			"server_port":3001,
			"cipher":"2022-blake3-aes-256-gcm",
			"server_key":"server-key",
			"network":"tcp,udp",
			"base_config":{"pull_interval":60,"push_interval":60}
		}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	gotType, err := generateXboardConfig(context.Background(), xboardGenerateOptions{
		PanelURL:    srv.URL,
		PanelToken:  "tok",
		PanelNodeID: "31",
		NodeMode:    "ss",
		Out:         out,
	})
	if err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}
	if gotType != "ss" {
		t.Fatalf("node type = %q, want ss", gotType)
	}
	opts, err := loadOptions(out)
	if err != nil {
		data, _ := os.ReadFile(out)
		t.Fatalf("generated SS config not loadable by sing-box: %v\n%s", err, data)
	}
	if len(opts.Inbounds) != 1 || opts.Inbounds[0].Type != "shadowsocks" {
		t.Fatalf("inbounds = %+v", opts.Inbounds)
	}
}

func TestGenerateConfigOmitsDeprecatedDNSOutbound(t *testing.T) {
	data, err := buildSingBoxConfigFromInbounds([]any{
		map[string]any{
			"type":        "vless",
			"tag":         "vless-in",
			"listen":      "0.0.0.0",
			"listen_port": 10001,
			"users":       []any{},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildSingBoxConfigFromInbounds: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("generated config is not json: %v", err)
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		t.Fatalf("outbounds missing: %#v", cfg["outbounds"])
	}
	for _, item := range outbounds {
		outbound, _ := item.(map[string]any)
		if outbound["type"] == "dns" || outbound["tag"] == "dns-out" {
			t.Fatalf("deprecated dns outbound should not be generated: %#v", outbound)
		}
	}
}

func TestGenerateConfigIncludesCustomDNSAndRouteOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"protocol":"shadowsocks",
			"listen_ip":"0.0.0.0",
			"server_port":3001,
			"cipher":"2022-blake3-aes-256-gcm",
			"server_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=",
			"network":"tcp,udp",
			"custom_dns":{
				"servers":[{"type":"local","tag":"local"}],
				"strategy":"ipv6_only"
			},
			"custom_outbounds":[
				{
					"tag":"direct-v6",
					"protocol":"direct",
					"settings":{"domain_resolver":{"server":"local","strategy":"ipv6_only"}}
				}
			],
			"route_options":{
				"default_domain_resolver":{"server":"local","strategy":"ipv6_only"},
				"final":"direct-v6"
			}
		}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	if _, err := generateXboardConfig(context.Background(), xboardGenerateOptions{
		PanelURL:    srv.URL,
		PanelToken:  "tok",
		PanelNodeID: "31",
		NodeMode:    "ss",
		Out:         out,
	}); err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("generated config is not json: %v\n%s", err, data)
	}
	dns, ok := cfg["dns"].(map[string]any)
	if !ok {
		t.Fatalf("dns missing: %#v", cfg["dns"])
	}
	if dns["strategy"] != "ipv6_only" {
		t.Fatalf("dns.strategy = %#v, want ipv6_only", dns["strategy"])
	}
	route, ok := cfg["route"].(map[string]any)
	if !ok {
		t.Fatalf("route missing: %#v", cfg["route"])
	}
	if route["final"] != "direct-v6" {
		t.Fatalf("route.final = %#v, want direct-v6", route["final"])
	}
	if _, ok := route["default_domain_resolver"]; !ok {
		t.Fatalf("route.default_domain_resolver missing: %#v", route)
	}
	if _, err := loadOptions(out); err != nil {
		t.Fatalf("generated config not loadable by sing-box: %v\n%s", err, data)
	}
}

func TestGenerateConfigIncludesCustomOutboundsAndRoutes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"protocol":"vless",
			"listen_ip":"0.0.0.0",
			"server_port":10001,
			"network":"tcp",
			"flow":"xtls-rprx-vision",
			"tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv","short_id":"sid"},
			"custom_outbounds":[
				{
					"tag":"relay-socks",
					"protocol":"socks",
					"settings":{"server":"1.2.3.4","server_port":1080,"version":"5"}
				},
				{
					"tag":"relay-chain",
					"protocol":"vless",
					"settings":{"server":"example.com","server_port":443,"uuid":"11111111-1111-1111-1111-111111111111"},
					"proxy_tag":"relay-socks"
				}
			],
			"custom_route_rules":[
				{
					"name":"route-example",
					"match":{"domain_suffixes":["example.com"],"ports":["443"]},
					"action":{"type":"route","target":"relay-chain"}
				},
				{
					"name":"block-ads",
					"match":{"domains":["ads.example.com"]},
					"action":{"type":"block"}
				}
			]
		}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	if _, err := generateXboardConfig(context.Background(), xboardGenerateOptions{
		PanelURL:    srv.URL,
		PanelToken:  "tok",
		PanelNodeID: "21",
		NodeMode:    "vless",
		Out:         out,
	}); err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("generated config is not json: %v", err)
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		t.Fatalf("outbounds missing: %#v", cfg["outbounds"])
	}
	if len(outbounds) != 4 {
		t.Fatalf("outbounds len = %d, want 4", len(outbounds))
	}
	foundChain := false
	for _, item := range outbounds {
		m, _ := item.(map[string]any)
		if m["tag"] == "relay-chain" {
			foundChain = true
			if m["type"] != "vless" {
				t.Fatalf("relay-chain type = %#v", m["type"])
			}
			if m["detour"] != "relay-socks" {
				t.Fatalf("relay-chain detour = %#v", m["detour"])
			}
		}
	}
	if !foundChain {
		t.Fatalf("relay-chain outbound not found in %#v", outbounds)
	}

	route, ok := cfg["route"].(map[string]any)
	if !ok {
		t.Fatalf("route missing: %#v", cfg["route"])
	}
	rules, ok := route["rules"].([]any)
	if !ok || len(rules) != 2 {
		t.Fatalf("route.rules = %#v", route["rules"])
	}
	first, _ := rules[0].(map[string]any)
	if first["outbound"] != "relay-chain" {
		t.Fatalf("first route outbound = %#v", first["outbound"])
	}
	if _, ok := first["domain_suffix"]; !ok {
		t.Fatalf("first route domain_suffix missing: %#v", first)
	}
	second, _ := rules[1].(map[string]any)
	if second["outbound"] != "block" {
		t.Fatalf("second route outbound = %#v", second["outbound"])
	}

	if _, err := loadOptions(out); err != nil {
		t.Fatalf("generated config not loadable by sing-box: %v\n%s", err, data)
	}
}

func TestGenerateConfigRejectsUnknownRouteTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"protocol":"vless",
			"listen_ip":"0.0.0.0",
			"server_port":10001,
			"network":"tcp",
			"tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv","short_id":"sid"},
			"custom_route_rules":[
				{
					"name":"route-example",
					"match":{"domain_suffixes":["example.com"]},
					"action":{"type":"route","target":"missing-outbound"}
				}
			]
		}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	_, err := generateXboardConfig(context.Background(), xboardGenerateOptions{
		PanelURL:    srv.URL,
		PanelToken:  "tok",
		PanelNodeID: "21",
		NodeMode:    "vless",
		Out:         out,
	})
	if err == nil {
		t.Fatal("expected generateXboardConfig to fail")
	}
	if got := err.Error(); got == "" || !containsStringFragment(got, "unknown outbound") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsStringFragment(s, fragment string) bool {
	return strings.Contains(s, fragment)
}
