package main

import (
	"os"
	"runtime"
	"testing"
)

func TestConfigureGoMaxProcsRespectsExplicitEnvironment(t *testing.T) {
	oldValue, hadValue := os.LookupEnv("GOMAXPROCS")
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv("GOMAXPROCS", oldValue)
		} else {
			_ = os.Unsetenv("GOMAXPROCS")
		}
	})

	previous := runtime.GOMAXPROCS(3)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
	t.Setenv("GOMAXPROCS", "4")

	configureGoMaxProcs()

	if got := runtime.GOMAXPROCS(0); got != 3 {
		t.Fatalf("GOMAXPROCS = %d, want explicit runtime value to be preserved", got)
	}
}

func TestConfigureGoMaxProcsDefaultsToOneWhenUnset(t *testing.T) {
	oldValue, hadValue := os.LookupEnv("GOMAXPROCS")
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv("GOMAXPROCS", oldValue)
		} else {
			_ = os.Unsetenv("GOMAXPROCS")
		}
	})

	previous := runtime.GOMAXPROCS(3)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
	_ = os.Unsetenv("GOMAXPROCS")

	configureGoMaxProcs()

	if got := runtime.GOMAXPROCS(0); got != 1 {
		t.Fatalf("GOMAXPROCS = %d, want default small-machine value 1", got)
	}
}
