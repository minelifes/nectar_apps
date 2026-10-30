-- 0017: project env groups (variables shared by several apps of a project) and environments (a project
-- can be the staging/production copy of another one: origin_id points at the project it was cloned from).
CREATE TABLE env_groups (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  UNIQUE (project_id, name)
);
CREATE TABLE env_group_vars (
  group_id   INTEGER NOT NULL REFERENCES env_groups(id) ON DELETE CASCADE,
  key        TEXT NOT NULL,
  value_enc  TEXT NOT NULL,
  is_secret  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (group_id, key)
);
ALTER TABLE projects ADD COLUMN environment TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN origin_id INTEGER REFERENCES projects(id) ON DELETE SET NULL;
