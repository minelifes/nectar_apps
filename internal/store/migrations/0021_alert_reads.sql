-- 0021: alerts a user has marked as read — hidden from that user's dashboard and nav badge (still listed on
-- the Alerts page). Per user: one person reading an alert does not hide it for the rest of the team.
CREATE TABLE alert_reads (
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    alert_id INTEGER NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    read_at  INTEGER NOT NULL,
    PRIMARY KEY (user_id, alert_id)
);
CREATE INDEX alert_reads_alert ON alert_reads(alert_id);
