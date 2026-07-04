package main

import "testing"

func TestStatsDetailsPayloadIncludesAgentVersion(t *testing.T) {
	origVersion := agentVersion
	agentVersion = "v-test"
	t.Cleanup(func() { agentVersion = origVersion })

	payload := statsDetailsPayload(&Hook{}, false, false)

	if payload["agent_version"] != "v-test" {
		t.Fatalf("agent_version = %v, want v-test", payload["agent_version"])
	}
	if payload["agent_slug"] != agentSlug {
		t.Fatalf("agent_slug = %v, want %s", payload["agent_slug"], agentSlug)
	}
}
