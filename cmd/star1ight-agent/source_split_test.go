package main

import "testing"

func TestHookSourceInboundTagUsesSourceBucketWhenEnabled(t *testing.T) {
	classifier, err := ParseSourceBuckets([]string{"cnix=103.96.140.122/32", "nbix=114.111.176.34/32"})
	if err != nil {
		t.Fatalf("ParseSourceBuckets: %v", err)
	}
	h := &Hook{
		sourceClassifier: classifier,
		sourceTraffic:    true,
	}

	if got := h.sourceInboundTag("ss-in", "103.96.140.122:53000"); got != "ss-in@source=cnix" {
		t.Fatalf("cnix tag = %q, want ss-in@source=cnix", got)
	}
	if got := h.sourceInboundTag("ss-in", "114.111.176.34:53000"); got != "ss-in@source=nbix" {
		t.Fatalf("nbix tag = %q, want ss-in@source=nbix", got)
	}
}

func TestHookSourceInboundTagKeepsBaseTagWhenDisabled(t *testing.T) {
	classifier, err := ParseSourceBuckets([]string{"cnix=103.96.140.122/32"})
	if err != nil {
		t.Fatalf("ParseSourceBuckets: %v", err)
	}
	h := &Hook{sourceClassifier: classifier}

	if got := h.sourceInboundTag("ss-in", "103.96.140.122:53000"); got != "ss-in" {
		t.Fatalf("tag = %q, want ss-in", got)
	}
}

func TestParseSourceBucketSpecsSupportsShellSafePlusSeparator(t *testing.T) {
	got := parseSourceBucketSpecs("cnix=103.96.140.122/32,138.252.163.34/32+nbix=114.111.176.34/32")
	want := []string{
		"cnix=103.96.140.122/32,138.252.163.34/32",
		"nbix=114.111.176.34/32",
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("spec[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
