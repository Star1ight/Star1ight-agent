package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestDiffUsersSpeedLimitOnlyChangeDoesNotMutateInbounds(t *testing.T) {
	m := NewUserManager(0)
	initial := panelapi.User{ID: 7, UUID: "uuid-7", Password: "pw-7", Name: "name-7", SpeedLimit: 10}
	if err := m.ApplyBox(nil, []panelapi.User{initial}); err != nil {
		t.Fatal(err)
	}

	changed := initial
	changed.SpeedLimit = 20
	adds, dels := m.diffUsers([]panelapi.User{changed})
	if adds != 0 || dels != 0 {
		t.Fatalf("speed-limit-only change must not mutate inbounds, got adds=%d dels=%d", adds, dels)
	}
}

func TestApplyBoxSpeedLimitOnlyChangeUpdatesLimiterRate(t *testing.T) {
	m := NewUserManager(0)
	initial := panelapi.User{ID: 7, UUID: "uuid-7", Password: "pw-7", Name: "name-7", SpeedLimit: 10}
	if err := m.ApplyBox(nil, []panelapi.User{initial}); err != nil {
		t.Fatal(err)
	}

	_, _, userRead, userWrite := m.DirectionalLimiters("uuid-7")
	if userRead == nil || userWrite == nil {
		t.Fatal("expected user limiters for initial speed limit")
	}
	if got, want := userRead.Rate(), mbpsToBytes(10); got != want {
		t.Fatalf("initial read rate = %d, want %d", got, want)
	}
	if got, want := userWrite.Rate(), mbpsToBytes(10); got != want {
		t.Fatalf("initial write rate = %d, want %d", got, want)
	}

	changed := initial
	changed.SpeedLimit = 20
	if err := m.ApplyBox(nil, []panelapi.User{changed}); err != nil {
		t.Fatal(err)
	}

	_, _, userRead, userWrite = m.DirectionalLimiters("uuid-7")
	if userRead == nil || userWrite == nil {
		t.Fatal("expected user limiters after speed limit update")
	}
	if got, want := userRead.Rate(), mbpsToBytes(20); got != want {
		t.Fatalf("updated read rate = %d, want %d", got, want)
	}
	if got, want := userWrite.Rate(), mbpsToBytes(20); got != want {
		t.Fatalf("updated write rate = %d, want %d", got, want)
	}
}
