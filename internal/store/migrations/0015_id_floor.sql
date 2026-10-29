-- 0015: ids of deleted apps and add-ons are never handed out again, and their backup plans go with them.
-- SQLite reuses the highest rowid after a delete, but these ids tag backups in shared restic
-- repositories ("app:<id>"), metric series and build history: a new app (maybe in another project)
-- must not inherit the snapshots of a deleted one. Inserts take max(id, floor) + 1.
CREATE TABLE id_floor (
    tbl TEXT    PRIMARY KEY,
    seq INTEGER NOT NULL
);

-- seed with everything an id may already have been used for (apps and add-ons deleted before this)
INSERT INTO id_floor (tbl, seq) VALUES ('apps', max(
    coalesce((SELECT max(id) FROM apps), 0),
    coalesce((SELECT max(subject_id) FROM backup_runs WHERE scope = 'app'), 0),
    coalesce((SELECT max(subject_id) FROM backup_plans WHERE scope = 'app'), 0),
    coalesce((SELECT max(CAST(substr(series, 5) AS INTEGER)) FROM metric_points WHERE series LIKE 'app:%'), 0)));
INSERT INTO id_floor (tbl, seq) VALUES ('addons', max(
    coalesce((SELECT max(id) FROM addons), 0),
    coalesce((SELECT max(subject_id) FROM backup_runs WHERE scope = 'addon'), 0),
    coalesce((SELECT max(subject_id) FROM backup_plans WHERE scope = 'addon'), 0)));

-- the triggers also run for apps and add-ons removed with their project (ON DELETE CASCADE), which
-- used to leave backup plans that failed every night
CREATE TRIGGER apps_id_floor AFTER DELETE ON apps BEGIN
    INSERT INTO id_floor (tbl, seq) VALUES ('apps', OLD.id) ON CONFLICT (tbl) DO UPDATE SET seq = max(seq, excluded.seq);
    DELETE FROM backup_plans WHERE scope = 'app' AND subject_id = OLD.id;
END;
CREATE TRIGGER addons_id_floor AFTER DELETE ON addons BEGIN
    INSERT INTO id_floor (tbl, seq) VALUES ('addons', OLD.id) ON CONFLICT (tbl) DO UPDATE SET seq = max(seq, excluded.seq);
    DELETE FROM backup_plans WHERE scope = 'addon' AND subject_id = OLD.id;
END;

-- plans left behind by project deletes before this migration
DELETE FROM backup_plans WHERE scope = 'app' AND subject_id NOT IN (SELECT id FROM apps);
DELETE FROM backup_plans WHERE scope = 'addon' AND subject_id NOT IN (SELECT id FROM addons);
