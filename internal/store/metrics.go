package store

import (
	"context"
	"database/sql"
	"time"
)

// Resolutions of the metric history.
const (
	MetricRaw    = 15  // seconds, kept MetricRawKeep
	MetricRollup = 900 // 15 minutes, kept MetricRollupKeep
)

const (
	MetricRawKeep    = 24 * time.Hour
	MetricRollupKeep = 30 * 24 * time.Hour
)

// MetricSample is one measurement of a series.
type MetricSample struct {
	Series    string
	CPUm      int64
	MemB      int64
	DiskUsed  int64
	DiskTotal int64
	RxBps     int64
	TxBps     int64
	Conns     int64 // databases: open client connections
	ConnsMax  int64 // databases: the server's connection limit (0 = not measured)
	Pods      int64 // apps/databases: pods the sample covers
	CPUPeak   int64 // the busiest pod's CPU (millicores)
	MemPeak   int64 // the busiest pod's memory (bytes)
}

// MetricPoint is an averaged point of a series.
type MetricPoint struct {
	T         time.Time `json:"t"`
	CPUm      int64     `json:"cpu_m"`
	MemB      int64     `json:"mem_b"`
	DiskUsed  int64     `json:"disk_used,omitempty"`
	DiskTotal int64     `json:"disk_total,omitempty"`
	RxBps     int64     `json:"rx_bps,omitempty"`
	TxBps     int64     `json:"tx_bps,omitempty"`
	Conns     int64     `json:"conns,omitempty"`
	ConnsMax  int64     `json:"conns_max,omitempty"`
}

// AddMetricSamples stores samples taken at t as raw points and adds them to the 15-minute rollups.
func (s *Store) AddMetricSamples(ctx context.Context, t time.Time, samples []MetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	raw := t.Unix() / MetricRaw * MetricRaw
	roll := t.Unix() / MetricRollup * MetricRollup
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, m := range samples {
			if _, err := tx.ExecContext(ctx, `INSERT INTO metric_points (series, res, ts, n, cpu_m, mem_b, disk_used, disk_total, rx_bps, tx_bps, conns, conns_max, pods, cpu_peak, mem_peak)
				VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(series, res, ts) DO UPDATE SET n = 1, cpu_m = excluded.cpu_m, mem_b = excluded.mem_b,
					disk_used = excluded.disk_used, disk_total = excluded.disk_total, rx_bps = excluded.rx_bps, tx_bps = excluded.tx_bps,
					conns = excluded.conns, conns_max = excluded.conns_max, pods = excluded.pods, cpu_peak = excluded.cpu_peak, mem_peak = excluded.mem_peak`,
				m.Series, MetricRaw, raw, m.CPUm, m.MemB, m.DiskUsed, m.DiskTotal, m.RxBps, m.TxBps, m.Conns, m.ConnsMax, m.Pods, m.CPUPeak, m.MemPeak); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO metric_points (series, res, ts, n, cpu_m, mem_b, disk_used, disk_total, rx_bps, tx_bps, conns, conns_max, pods, cpu_peak, mem_peak)
				VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(series, res, ts) DO UPDATE SET n = n + 1, cpu_m = cpu_m + excluded.cpu_m, mem_b = mem_b + excluded.mem_b,
					disk_used = disk_used + excluded.disk_used, disk_total = disk_total + excluded.disk_total,
					rx_bps = rx_bps + excluded.rx_bps, tx_bps = tx_bps + excluded.tx_bps,
					conns = conns + excluded.conns, conns_max = conns_max + excluded.conns_max, pods = pods + excluded.pods,
					cpu_peak = MAX(cpu_peak, excluded.cpu_peak), mem_peak = MAX(mem_peak, excluded.mem_peak)`,
				m.Series, MetricRollup, roll, m.CPUm, m.MemB, m.DiskUsed, m.DiskTotal, m.RxBps, m.TxBps, m.Conns, m.ConnsMax, m.Pods, m.CPUPeak, m.MemPeak); err != nil {
				return err
			}
		}
		return nil
	})
}

// MetricHistory returns averaged points of a series in [from, to], at most about maxPoints of them.
// Ranges up to 24 h use the raw samples, longer ones the 15-minute rollups.
func (s *Store) MetricHistory(ctx context.Context, series string, from, to time.Time, maxPoints int) ([]MetricPoint, error) {
	if maxPoints <= 0 {
		maxPoints = 240
	}
	res := int64(MetricRaw)
	if to.Sub(from) > MetricRawKeep {
		res = MetricRollup
	}
	step := int64(to.Sub(from).Seconds()) / int64(maxPoints)
	if step < res {
		step = res
	}
	step = (step + res - 1) / res * res
	rows, err := s.db.QueryContext(ctx, `SELECT (ts / ?) * ? AS b, SUM(n), SUM(cpu_m), SUM(mem_b), SUM(disk_used), SUM(disk_total), SUM(rx_bps), SUM(tx_bps), SUM(conns), SUM(conns_max)
		FROM metric_points WHERE series = ? AND res = ? AND ts >= ? AND ts <= ?
		GROUP BY b ORDER BY b`, step, step, series, res, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetricPoint{}
	for rows.Next() {
		var b, n, cpu, mem, du, dt, rx, txb, cn, cm int64
		if err := rows.Scan(&b, &n, &cpu, &mem, &du, &dt, &rx, &txb, &cn, &cm); err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}
		out = append(out, MetricPoint{T: time.Unix(b, 0).UTC(), CPUm: cpu / n, MemB: mem / n, DiskUsed: du / n, DiskTotal: dt / n, RxBps: rx / n, TxBps: txb / n,
			Conns: (cn + n/2) / n, ConnsMax: cm / n})
	}
	return out, rows.Err()
}

// UsagePoint is one 15-minute rollup of an app's pods: average use per pod and the busiest pod's peak.
type UsagePoint struct {
	T          time.Time
	CPUPerPod  int64 // millicores, averaged over the pods and the 15 minutes
	MemPerPod  int64 // bytes
	CPUPeak    int64 // the busiest pod at its busiest sample
	MemPeak    int64
	PodsAvg    float64
	HasSamples bool
}

// UsageSince returns the 15-minute rollups of a series since from, only the buckets that saw pods.
func (s *Store) UsageSince(ctx context.Context, series string, from time.Time) ([]UsagePoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, n, cpu_m, mem_b, pods, cpu_peak, mem_peak FROM metric_points
		WHERE series = ? AND res = ? AND ts >= ? AND pods > 0 ORDER BY ts`, series, MetricRollup, from.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsagePoint
	for rows.Next() {
		var ts, n, cpu, mem, pods, cpuPeak, memPeak int64
		if err := rows.Scan(&ts, &n, &cpu, &mem, &pods, &cpuPeak, &memPeak); err != nil {
			return nil, err
		}
		// cpu and mem are sums over the samples of the app's total; pods the sum of the pod counts: the
		// ratio is the average use of one pod
		out = append(out, UsagePoint{T: time.Unix(ts, 0).UTC(), CPUPerPod: cpu / pods, MemPerPod: mem / pods,
			CPUPeak: cpuPeak, MemPeak: memPeak, PodsAvg: float64(pods) / float64(max(n, 1)), HasSamples: true})
	}
	return out, rows.Err()
}

// PruneMetrics drops raw points older than 24 h and rollups older than 30 days, and series that no
// longer exist are left to age out the same way.
func (s *Store) PruneMetrics(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM metric_points WHERE (res = ? AND ts < ?) OR (res = ? AND ts < ?)`,
		MetricRaw, now.Add(-MetricRawKeep).Unix(), MetricRollup, now.Add(-MetricRollupKeep).Unix())
	return err
}
