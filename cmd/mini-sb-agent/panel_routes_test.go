package main

import "testing"

func TestParsePanelRoutesAcceptsRepeatedProtocolNodePairs(t *testing.T) {
	routes, err := parsePanelRoutes([]string{"vless:vless-tcp:40", "hysteria:hy2-udp:41"}, "http://panel", "token")
	if err != nil {
		t.Fatalf("parse panel routes: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("routes = %d, want 2", len(routes))
	}
	if routes[0].nodeType != "vless" || routes[0].inboundTag != "vless-tcp" || routes[0].nodeID != "40" {
		t.Fatalf("first route = %#v", routes[0])
	}
	if routes[1].nodeType != "hysteria" || routes[1].inboundTag != "hy2-udp" || routes[1].nodeID != "41" {
		t.Fatalf("second route = %#v", routes[1])
	}
}

func TestParsePanelRoutesRejectsMalformedRoute(t *testing.T) {
	if _, err := parsePanelRoutes([]string{"vless:40"}, "http://panel", "token"); err == nil {
		t.Fatal("parsePanelRoutes accepted malformed route, want error")
	}
}

func TestParsePanelRoutesFallsBackToLegacySingleNode(t *testing.T) {
	routes, err := parsePanelRoutes(nil, "http://panel", "token", legacyPanelRoute{
		nodeType: "vless",
		nodeID:   "40",
	})
	if err != nil {
		t.Fatalf("parse legacy panel route: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(routes))
	}
	if routes[0].nodeType != "vless" || routes[0].nodeID != "40" || len(routes[0].inboundTags) != 0 {
		t.Fatalf("legacy route = %#v", routes[0])
	}
}

func TestPanelRoutesForSyncerFetchesUsersFromFirstRouteOnly(t *testing.T) {
	routes, err := parsePanelRoutes([]string{"vless:vless-tcp:40", "hysteria:hy2-udp:41"}, "http://panel", "token")
	if err != nil {
		t.Fatalf("parse panel routes: %v", err)
	}
	got := panelRoutesForSyncer(routes)
	if len(got) != 2 {
		t.Fatalf("syncer routes = %d, want 2", len(got))
	}
	if !got[0].FetchUsers {
		t.Fatal("first panel route should fetch users")
	}
	if got[1].FetchUsers {
		t.Fatal("second panel route should not fetch users")
	}
	if len(got[0].InboundTags) != 1 || got[0].InboundTags[0] != "vless-tcp" {
		t.Fatalf("first inbound tags = %#v", got[0].InboundTags)
	}
	if len(got[1].InboundTags) != 1 || got[1].InboundTags[0] != "hy2-udp" {
		t.Fatalf("second inbound tags = %#v", got[1].InboundTags)
	}
}
