package sizing

import (
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/store"
)

// pts makes hours of 15-minute points with a per-pod use and peaks.
func pts(hours int, cpu, memMB, cpuPeak, memPeakMB int64) []store.UsagePoint {
	var out []store.UsagePoint
	for i := range hours * 4 {
		out = append(out, store.UsagePoint{T: time.Unix(int64(i*900), 0), CPUPerPod: cpu, MemPerPod: memMB << 20,
			CPUPeak: cpuPeak, MemPeak: memPeakMB << 20, PodsAvg: 1, HasSamples: true})
	}
	return out
}

func kinds(r Report) string {
	var k []string
	for _, h := range r.Hints {
		k = append(k, h.Kind)
	}
	return strings.Join(k, ",")
}

func TestAnalyze(t *testing.T) {
	// not enough data yet
	if r := Analyze(store.Resources{CPUMillicores: 500, MemoryMB: 1024}, 1, pts(10, 20, 180, 50, 210)); r.Ready || len(r.Hints) != 0 {
		t.Fatalf("10 h of data: %+v", r)
	}
	// 1 GB reserved, ~180 MB used, peak 210: suggest 288 MB (210 × 1.25 → 263 → 288), 2 replicas free 1472 MB
	r := Analyze(store.Resources{CPUMillicores: 1000, MemoryMB: 1024}, 2, pts(48, 40, 180, 120, 210))
	if !r.Ready || r.MemP95MB != 180 || r.MemPeakMB != 210 || r.SuggestMem != 288 || r.SavingMemMB != 2*(1024-288) ||
		r.SuggestCPU != 100 || r.SavingCPU != 2*900 || kinds(r) != "memory_too_high,cpu_too_high" {
		t.Fatalf("over-provisioned: %+v", r)
	}
	if !strings.Contains(r.Hints[0].Message, "Reserves 1.0 GB, uses 180 MB (peak 210 MB): 288 MB would be enough") {
		t.Fatalf("message: %s", r.Hints[0].Message)
	}
	// close to the memory limit, at the CPU limit
	r = Analyze(store.Resources{CPUMillicores: 250, MemoryMB: 256}, 1, pts(48, 240, 200, 250, 240))
	if kinds(r) != "memory_too_low,cpu_too_low" || r.SuggestMem != 320 || r.SavingMemMB >= 0 {
		t.Fatalf("too low: %+v", r)
	}
	// no limit: being near the request is fine
	if r := Analyze(store.Resources{CPUMillicores: 250, MemoryMB: 256, NoLimit: true}, 1, pts(48, 240, 200, 250, 240)); kinds(r) != "" {
		t.Fatalf("no limit: %+v", r)
	}
	// well sized: no hint
	if r := Analyze(store.Resources{CPUMillicores: 200, MemoryMB: 384}, 1, pts(48, 100, 200, 180, 260)); kinds(r) != "" {
		t.Fatalf("well sized: %+v", r)
	}
	// nothing reserved
	if r := Analyze(store.Resources{}, 1, pts(48, 150, 200, 300, 260)); kinds(r) != "no_request,no_request" {
		t.Fatalf("no request: %+v", r)
	}
	// the 95th percentile ignores a few busy quarter hours
	busy := pts(48, 20, 100, 900, 120)
	for i := 0; i < 5; i++ {
		busy[i].CPUPerPod = 900
	}
	if r := Analyze(store.Resources{CPUMillicores: 1000, MemoryMB: 256}, 1, busy); r.CPUP95m != 20 || r.CPUPeakm != 900 {
		t.Fatalf("p95: %+v", r)
	}
}
