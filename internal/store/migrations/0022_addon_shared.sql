-- 0022: a database shared with the whole project: every app of the project gets its connection variables
-- (shared_prefix + DATABASE_URL, …) without a link per app. An explicit link of an app wins (its own prefix).
ALTER TABLE addons ADD COLUMN shared INTEGER NOT NULL DEFAULT 0;
ALTER TABLE addons ADD COLUMN shared_prefix TEXT NOT NULL DEFAULT '';
