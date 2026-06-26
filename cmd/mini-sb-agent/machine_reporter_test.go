package main

import (
	"context"
	"errors"
	"testing"

	"mini-sb-agent/panelapi"
)

type stubMachineReporter struct {
	last panelapi.MachineStatus
	err  error
	hits int
}

func (s *stubMachineReporter) ReportMachineStatus(ctx context.Context, status panelapi.MachineStatus) error {
	s.hits++
	s.last = status
	return s.err
}

func TestSampleMachineStatusCollectsRequiredMetrics(t *testing.T) {
	origMeminfo := readUintFromMeminfoFunc
	origDiskUsage := diskUsageFunc
	t.Cleanup(func() {
		readUintFromMeminfoFunc = origMeminfo
		diskUsageFunc = origDiskUsage
	})
	readUintFromMeminfoFunc = func(key string) uint64 {
		switch key {
		case "MemTotal:":
			return 4096
		case "MemAvailable:":
			return 1024
		case "SwapTotal:":
			return 2048
		case "SwapFree:":
			return 512
		default:
			return 0
		}
	}
	diskUsageFunc = func(path string) (uint64, uint64) {
		return 1000, 250
	}

	status := sampleMachineStatus(func() (float64, error) { return 37.5, nil })
	if status.CPU != 37.5 {
		t.Fatalf("CPU = %v, want 37.5", status.CPU)
	}
	if status.Mem.Total != 4096*1024 || status.Mem.Used != 3072*1024 {
		t.Fatalf("Mem = %+v", status.Mem)
	}
	if status.Swap.Total != 2048*1024 || status.Swap.Used != 1536*1024 {
		t.Fatalf("Swap = %+v", status.Swap)
	}
	if status.Disk.Total != 1000 || status.Disk.Used != 250 {
		t.Fatalf("Disk = %+v", status.Disk)
	}
}

func TestSampleMachineStatusReturnsCPUZeroOnCollectorError(t *testing.T) {
	status := sampleMachineStatus(func() (float64, error) { return 0, errors.New("boom") })
	if status.CPU != 0 {
		t.Fatalf("CPU = %v, want 0", status.CPU)
	}
}

func TestMachineReporterTickReportsStatus(t *testing.T) {
	reporter := &stubMachineReporter{}
	err := machineReporterTick(context.Background(), reporter, func() panelapi.MachineStatus {
		return panelapi.MachineStatus{
			CPU: 12.5,
			Mem: panelapi.UsagePair{Total: 100, Used: 50},
		}
	})
	if err != nil {
		t.Fatalf("machineReporterTick: %v", err)
	}
	if reporter.hits != 1 {
		t.Fatalf("hits = %d, want 1", reporter.hits)
	}
	if reporter.last.CPU != 12.5 {
		t.Fatalf("CPU = %v, want 12.5", reporter.last.CPU)
	}
}

func TestMachineReporterTickReturnsReporterError(t *testing.T) {
	want := errors.New("panel down")
	reporter := &stubMachineReporter{err: want}
	err := machineReporterTick(context.Background(), reporter, func() panelapi.MachineStatus {
		return panelapi.MachineStatus{
			CPU: 1,
			Mem: panelapi.UsagePair{Total: 1, Used: 1},
		}
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestBuildMachineReporterRequiresMachineToken(t *testing.T) {
	reporter, err := buildMachineReporter("https://panel.example.com", "17", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if reporter != nil {
		t.Fatalf("reporter = %#v, want nil", reporter)
	}
}

func TestBuildMachineReporterUsesDedicatedMachineToken(t *testing.T) {
	reporter, err := buildMachineReporter("https://panel.example.com", "17", "machine-token")
	if err != nil {
		t.Fatalf("buildMachineReporter: %v", err)
	}
	client, ok := reporter.(*panelapi.Client)
	if !ok {
		t.Fatalf("reporter type = %T, want *panelapi.Client", reporter)
	}
	if client.MachineID != "17" {
		t.Fatalf("MachineID = %q, want 17", client.MachineID)
	}
	if client.MachineToken != "machine-token" {
		t.Fatalf("MachineToken = %q", client.MachineToken)
	}
}
