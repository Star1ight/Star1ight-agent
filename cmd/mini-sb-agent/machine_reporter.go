package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mini-sb-agent/panelapi"
)

func buildMachineReporter(panelURL, machineID, machineToken string) (panelapi.MachineReporter, error) {
	if machineID == "" {
		return nil, nil
	}
	if machineToken == "" {
		return nil, fmt.Errorf("machine-id requires machine-token")
	}
	client := panelapi.NewClient(panelURL, "", "", "")
	client.MachineID = machineID
	client.MachineToken = machineToken
	return client, nil
}

var (
	readUintFromMeminfoFunc = readUintFromMeminfo
	diskUsageFunc           = diskUsage
)

type cpuCollector struct {
	prevTotal uint64
	prevIdle  uint64
	ready     bool
}

func (c *cpuCollector) Sample() (float64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			break
		}
		var values []uint64
		for _, field := range fields[1:] {
			v, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return 0, err
			}
			values = append(values, v)
		}
		var total uint64
		for _, v := range values {
			total += v
		}
		idle := values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		if !c.ready {
			c.prevTotal = total
			c.prevIdle = idle
			c.ready = true
			return 0, nil
		}
		deltaTotal := total - c.prevTotal
		deltaIdle := idle - c.prevIdle
		c.prevTotal = total
		c.prevIdle = idle
		if deltaTotal == 0 {
			return 0, nil
		}
		return float64(deltaTotal-deltaIdle) * 100 / float64(deltaTotal), nil
	}
	return 0, nil
}

func sampleMachineStatus(cpuSample func() (float64, error)) panelapi.MachineStatus {
	cpu, err := cpuSample()
	if err != nil {
		cpu = 0
	}
	totalMem := readUintFromMeminfoFunc("MemTotal:")
	availableMem := readUintFromMeminfoFunc("MemAvailable:")
	memUsed := uint64(0)
	if totalMem >= availableMem {
		memUsed = totalMem - availableMem
	}
	swapTotal := readUintFromMeminfoFunc("SwapTotal:")
	swapFree := readUintFromMeminfoFunc("SwapFree:")
	swapUsed := uint64(0)
	if swapTotal >= swapFree {
		swapUsed = swapTotal - swapFree
	}
	diskTotal, diskUsed := diskUsageFunc("/")

	return panelapi.MachineStatus{
		CPU: cpu,
		Mem: panelapi.UsagePair{
			Total: totalMem * 1024,
			Used:  memUsed * 1024,
		},
		Swap: panelapi.UsagePair{
			Total: swapTotal * 1024,
			Used:  swapUsed * 1024,
		},
		Disk: panelapi.UsagePair{
			Total: diskTotal,
			Used:  diskUsed,
		},
	}
}

func machineReporterTick(ctx context.Context, reporter panelapi.MachineReporter, sampler func() panelapi.MachineStatus) error {
	if reporter == nil || sampler == nil {
		return nil
	}
	return reporter.ReportMachineStatus(ctx, sampler())
}

func startMachineReporter(ctx context.Context, reporter panelapi.MachineReporter, every time.Duration) {
	if reporter == nil {
		return
	}
	if every <= 0 {
		every = time.Minute
	}
	cpu := &cpuCollector{}
	sampler := func() panelapi.MachineStatus {
		return sampleMachineStatus(cpu.Sample)
	}

	go func() {
		if err := machineReporterTick(ctx, reporter, sampler); err != nil && ctx.Err() == nil {
			log.Printf("panel api machine status: %v", err)
		}
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := machineReporterTick(ctx, reporter, sampler); err != nil && ctx.Err() == nil {
					log.Printf("panel api machine status: %v", err)
				}
			}
		}
	}()
}

func readUintFromMeminfo(prefix string) uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	return 0
}

func diskUsage(path string) (total uint64, used uint64) {
	if path == "" {
		path = "/"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	var fs syscallStatfs
	if err := statfs(abs, &fs); err != nil {
		return 0, 0
	}
	total = fs.blocks * uint64(fs.bsize)
	free := fs.bavail * uint64(fs.bsize)
	if total >= free {
		used = total - free
	}
	return total, used
}
