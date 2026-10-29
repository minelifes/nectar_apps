-- 0005: projects can be created from an existing namespace (adopt mode, PLAN.md §2b).
-- An adopted project keeps its namespace name; the panel manages only the objects it creates there and
-- never deletes the namespace.

ALTER TABLE projects ADD COLUMN adopted INTEGER NOT NULL DEFAULT 0;
