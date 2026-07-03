package main

import (
	"testing"

	"star1ight-agent/panelapi"
)

func TestLoadRuntimeOptionsMachineOnlySkipsMissingConfig(t *testing.T) {
	opts, err := loadRuntimeOptions("/path/does/not/exist.json", hy2Tuning{}, true)
	if err != nil {
		t.Fatalf("loadRuntimeOptions returned error in machine-only mode: %v", err)
	}
	if opts != nil {
		t.Fatalf("loadRuntimeOptions returned %#v, want nil in machine-only mode", opts)
	}
}

func TestLoadRuntimeOptionsRequiresConfigOutsideMachineOnly(t *testing.T) {
	opts, err := loadRuntimeOptions("/path/does/not/exist.json", hy2Tuning{}, false)
	if err == nil {
		t.Fatal("expected missing-config error outside machine-only mode")
	}
	if opts != nil {
		t.Fatalf("loadRuntimeOptions returned %#v, want nil on error", opts)
	}
}

func TestConfigurePanelMachineOnlyBuildsReporterWithoutNodeSync(t *testing.T) {
	panel, reporter, err := configurePanel(
		"https://panel.example.com",
		"panel-token",
		"",
		"vless",
		"",
		"hysteria",
		"",
		"17",
		"machine-token",
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("configurePanel: %v", err)
	}
	if panel != nil {
		t.Fatalf("panel = %#v, want nil in machine-only mode", panel)
	}
	client, ok := reporter.(*panelapi.Client)
	if !ok {
		t.Fatalf("reporter type = %T, want *panelapi.Client", reporter)
	}
	if client.MachineID != "17" || client.MachineToken != "machine-token" {
		t.Fatalf("reporter = %#v", client)
	}
}

func TestConfigurePanelBuildsNodeSyncClientWhenNodeIDPresent(t *testing.T) {
	panel, reporter, err := configurePanel(
		"https://panel.example.com",
		"panel-token",
		"21",
		"vless",
		"",
		"hysteria",
		"",
		"",
		"",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("configurePanel: %v", err)
	}
	if reporter != nil {
		t.Fatalf("reporter = %#v, want nil", reporter)
	}
	client, ok := panel.(*panelapi.Client)
	if !ok {
		t.Fatalf("panel type = %T, want *panelapi.Client", panel)
	}
	if client.NodeID != "21" || client.NodeType != "vless" {
		t.Fatalf("panel client = %#v", client)
	}
}

func TestConfigurePanelUsesLocalUsersWhenPanelURLMissing(t *testing.T) {
	panel, reporter, err := configurePanel(
		"",
		"",
		"",
		"vless",
		"",
		"hysteria",
		"/tmp/local-users.json",
		"",
		"",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("configurePanel: %v", err)
	}
	if reporter != nil {
		t.Fatalf("reporter = %#v, want nil", reporter)
	}
	local, ok := panel.(panelapi.LocalUsers)
	if !ok {
		t.Fatalf("panel type = %T, want panelapi.LocalUsers", panel)
	}
	if local.Path != "/tmp/local-users.json" {
		t.Fatalf("local users path = %q", local.Path)
	}
}
