-- 0026: database connections in the metric history (add-ons): open client connections and the server's
-- limit (0 = not measured).
ALTER TABLE metric_points ADD COLUMN conns INTEGER NOT NULL DEFAULT 0;
ALTER TABLE metric_points ADD COLUMN conns_max INTEGER NOT NULL DEFAULT 0;
