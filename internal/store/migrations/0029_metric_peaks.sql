-- 0029: per-pod figures for right-sizing: how many pods a sample covered, and the busiest pod's CPU and
-- memory (rollups keep the maximum, so a short peak survives the 15-minute average).
ALTER TABLE metric_points ADD COLUMN pods INTEGER NOT NULL DEFAULT 0;
ALTER TABLE metric_points ADD COLUMN cpu_peak INTEGER NOT NULL DEFAULT 0;
ALTER TABLE metric_points ADD COLUMN mem_peak INTEGER NOT NULL DEFAULT 0;
