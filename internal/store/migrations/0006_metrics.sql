-- 0006: resource usage history (PLAN.md §6). The collector samples every 15 s.
-- res = 15 → raw samples (kept 24 h); res = 900 → 15-minute averages (kept 30 days), kept as sum + count so
-- they can be updated with every new sample.
-- series: "node:<name>" or "app:<id>".
CREATE TABLE metric_points (
    series     TEXT    NOT NULL,
    res        INTEGER NOT NULL,
    ts         INTEGER NOT NULL, -- unix seconds, start of the bucket
    n          INTEGER NOT NULL DEFAULT 1,
    cpu_m      INTEGER NOT NULL DEFAULT 0, -- millicores (sum over n samples)
    mem_b      INTEGER NOT NULL DEFAULT 0, -- bytes (sum)
    disk_used  INTEGER NOT NULL DEFAULT 0, -- bytes (sum; nodes only)
    disk_total INTEGER NOT NULL DEFAULT 0, -- bytes (sum; nodes only)
    rx_bps     INTEGER NOT NULL DEFAULT 0, -- bytes/s received (sum; nodes only)
    tx_bps     INTEGER NOT NULL DEFAULT 0, -- bytes/s sent (sum)
    PRIMARY KEY (series, res, ts)
) WITHOUT ROWID;
