BEGIN;

LOCK TABLE projects IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM projects
        GROUP BY parse_result_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'duplicate projects exist for the same parse_result_id; resolve them before applying uq_projects_parse_result_id';
    END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_projects_parse_result_id
    ON projects(parse_result_id);

DROP INDEX IF EXISTS idx_projects_parse_result_id;

COMMIT;
