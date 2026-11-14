// Package sizing compares what an app reserves per replica (CPU and memory requests) with what its pods
// really use, from the stored 15-minute metrics, and suggests better values.
package sizing

import (
	"fmt"
	"math"
	"slices"
	"time"

	"nectar_apps/internal/store"
)

// Window is how far back usage is read; MinData is how much of it must exist before a hint is given.
const (
	Window  = 14 * 24 * time.Hour
	MinData = 24 * time.Hour
)

// Headroom on top of the measured use: memory above the busiest pod's peak (a pod over its limit is
// killed), CPU above the 95th percentile (more only slows a pod down, it is not killed).
const (
	memHeadroom = 1.25
	cpuHeadroom = 1.3
	memStepMB   = 32
	cpuStepM    = 50
	minMemMB    = 64
	minCPUm     = 50
)

// Hint kinds.
const (
	MemTooHigh = "memory_too_high" // reserves much more memory than it uses
	MemTooLow  = "memory_too_low"  // close to its memory limit: risk of being killed (OOM)
	CPUTooHigh = "cpu_too_high"
	CPUTooLow  = "cpu_too_low" // at its CPU limit most of the time: slowed down
	NoRequest  = "no_request"  // reserves nothing: the scheduler can't plan for it
)

// Hint is one finding.
type Hint struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Report is the sizing of one app (per replica).
type Report struct {
	// enough data to judge (MinData of pods running in Window)
	Ready      bool    `json:"ready"`
	DataHours  float64 `json:"data_hours"`
	ReqCPUm    int64   `json:"request_cpu_m"`
	ReqMemMB   int64   `json:"request_memory_mb"`
	Limited    bool    `json:"limited"` // the request is also the limit (not no_limit)
	CPUP95m    int64   `json:"cpu_p95_m"`
	CPUPeakm   int64   `json:"cpu_peak_m"`
	MemP95MB   int64   `json:"memory_p95_mb"`
	MemPeakMB  int64   `json:"memory_peak_mb"`
	SuggestCPU int64   `json:"suggest_cpu_m"`
	SuggestMem int64   `json:"suggest_memory_mb"`
	// SavingCPU / SavingMemMB: what the whole app (all replicas) would free with the suggestion (negative
	// = it needs more)
	SavingCPU   int64  `json:"saving_cpu_m"`
	SavingMemMB int64  `json:"saving_memory_mb"`
	Replicas    int    `json:"replicas"`
	Hints       []Hint `json:"hints"`
}

func roundUp(v float64, step int64) int64 {
	return int64(math.Ceil(v/float64(step))) * step
}

func p95(vs []int64) int64 {
	if len(vs) == 0 {
		return 0
	}
	s := slices.Clone(vs)
	slices.Sort(s)
	return s[int(math.Ceil(0.95*float64(len(s))))-1]
}

func mb(b int64) int64 { return (b + 1<<20 - 1) >> 20 }

// FormatMB writes 1536 as "1.5 GB" and 180 as "180 MB".
func FormatMB(v int64) string {
	if v >= 1024 {
		return fmt.Sprintf("%.1f GB", float64(v)/1024)
	}
	return fmt.Sprintf("%d MB", v)
}

func formatCPU(m int64) string {
	if m >= 1000 {
		return fmt.Sprintf("%.2g CPU", float64(m)/1000)
	}
	return fmt.Sprintf("%dm CPU", m)
}

// Analyze sizes an app from its usage points (store.UsageSince of its series).
func Analyze(res store.Resources, replicas int, pts []store.UsagePoint) Report {
	r := Report{ReqCPUm: res.CPUMillicores, ReqMemMB: res.MemoryMB, Limited: !res.NoLimit, Replicas: replicas, Hints: []Hint{}}
	r.DataHours = float64(len(pts)) * float64(store.MetricRollup) / 3600
	if time.Duration(r.DataHours*float64(time.Hour)) < MinData {
		return r
	}
	r.Ready = true
	cpus := make([]int64, 0, len(pts))
	mems := make([]int64, 0, len(pts))
	for _, p := range pts {
		cpus = append(cpus, p.CPUPerPod)
		mems = append(mems, p.MemPerPod)
		r.CPUPeakm = max(r.CPUPeakm, p.CPUPeak)
		r.MemPeakMB = max(r.MemPeakMB, mb(p.MemPeak))
	}
	r.CPUP95m, r.MemP95MB = p95(cpus), mb(p95(mems))
	r.SuggestMem = max(roundUp(float64(max(r.MemPeakMB, r.MemP95MB))*memHeadroom, memStepMB), minMemMB)
	r.SuggestCPU = max(roundUp(float64(r.CPUP95m)*cpuHeadroom, cpuStepM), minCPUm)
	n := int64(max(replicas, 1))

	add := func(kind, msg string) { r.Hints = append(r.Hints, Hint{Kind: kind, Message: msg}) }
	// memory: being killed is worse than wasting, so the "too low" check comes first
	switch {
	case res.MemoryMB == 0:
		add(NoRequest, fmt.Sprintf("Reserves no memory but uses up to %s: reserve %s so the server plans for it.", FormatMB(r.MemPeakMB), FormatMB(r.SuggestMem)))
	case !res.NoLimit && float64(r.MemPeakMB) >= 0.9*float64(res.MemoryMB):
		add(MemTooLow, fmt.Sprintf("Uses up to %s of its %s limit: near the limit a pod is killed (out of memory). Raise it to %s.",
			FormatMB(r.MemPeakMB), FormatMB(res.MemoryMB), FormatMB(r.SuggestMem)))
	case res.MemoryMB >= 2*r.SuggestMem && res.MemoryMB-r.SuggestMem >= 128:
		add(MemTooHigh, fmt.Sprintf("Reserves %s, uses %s (peak %s): %s would be enough.",
			FormatMB(res.MemoryMB), FormatMB(r.MemP95MB), FormatMB(r.MemPeakMB), FormatMB(r.SuggestMem)))
	}
	switch {
	case res.CPUMillicores == 0:
		if r.CPUP95m >= minCPUm {
			add(NoRequest, fmt.Sprintf("Reserves no CPU but uses %s most of the time: reserve %s.", formatCPU(r.CPUP95m), formatCPU(r.SuggestCPU)))
		}
	case !res.NoLimit && float64(r.CPUP95m) >= 0.9*float64(res.CPUMillicores):
		add(CPUTooLow, fmt.Sprintf("Runs at its %s limit most of the time, so it is slowed down. Raise it to %s.",
			formatCPU(res.CPUMillicores), formatCPU(r.SuggestCPU)))
	case res.CPUMillicores >= 2*r.SuggestCPU && res.CPUMillicores-r.SuggestCPU >= 200:
		add(CPUTooHigh, fmt.Sprintf("Reserves %s, uses %s (95th percentile; peak %s): %s would be enough.",
			formatCPU(res.CPUMillicores), formatCPU(r.CPUP95m), formatCPU(r.CPUPeakm), formatCPU(r.SuggestCPU)))
	}
	if res.MemoryMB > 0 {
		r.SavingMemMB = (res.MemoryMB - r.SuggestMem) * n
	}
	if res.CPUMillicores > 0 {
		r.SavingCPU = (res.CPUMillicores - r.SuggestCPU) * n
	}
	return r
}
