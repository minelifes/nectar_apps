-- 0023: an app marked as a database (an imported MySQL, a Postgres image…): engine, port and where its
-- credentials come from (names of its own variables). JSON, '' = not a database.
ALTER TABLE apps ADD COLUMN database_json TEXT NOT NULL DEFAULT '';
