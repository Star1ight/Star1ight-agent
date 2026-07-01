package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrandingDefaultsUseStar1ightAgentNames(t *testing.T) {
	if agentSlug != "star1ight-agent" {
		t.Fatalf("agentSlug = %q", agentSlug)
	}
	if defaultStatsAPI != "unix:/tmp/star1ight-agent.sock" {
		t.Fatalf("defaultStatsAPI = %q", defaultStatsAPI)
	}
	if runtimeSmokeEnv != "STAR1IGHT_AGENT_RUNTIME_SMOKE" {
		t.Fatalf("runtimeSmokeEnv = %q", runtimeSmokeEnv)
	}
	if runtimeSmokeBinEnv != "STAR1IGHT_AGENT_BIN" {
		t.Fatalf("runtimeSmokeBinEnv = %q", runtimeSmokeBinEnv)
	}
}

func TestInstallScriptUsesStar1ightAgentIdentifiers(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	body := string(data)

	mustContain := []string{
		`APP="star1ight-agent"`,
		`INSTALL_DIR="/opt/star1ight-agent"`,
		`RUN_DIR="/run/star1ight-agent"`,
		`SERVICE_NAME="star1ight-agent"`,
		`ASSET="star1ight-agent-linux-$ASSET_ARCH"`,
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Fatalf("install.sh missing %q", want)
		}
	}

	mustNotContain := []string{
		"/opt/mini-sb-agent",
		"/run/mini-sb-agent",
		"mini-sb-agent.sock",
		"MINI_SB_AGENT_",
	}
	for _, banned := range mustNotContain {
		if strings.Contains(body, banned) {
			t.Fatalf("install.sh still contains %q", banned)
		}
	}
}
