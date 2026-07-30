BEGIN;

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_projects_version'
          AND conrelid = 'projects'::regclass
    ) THEN
        ALTER TABLE projects
            ADD CONSTRAINT chk_projects_version CHECK (version >= 1);
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_tasks_version'
          AND conrelid = 'tasks'::regclass
    ) THEN
        ALTER TABLE tasks
            ADD CONSTRAINT chk_tasks_version CHECK (version >= 1);
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_projects_user_status_updated_at
    ON projects(user_id, status, updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_tasks_project_sort_order
    ON tasks(project_id, sort_order, id);

COMMIT;
