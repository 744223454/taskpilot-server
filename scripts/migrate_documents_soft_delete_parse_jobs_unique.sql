BEGIN;

ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_documents_deleted_at
    ON documents(deleted_at);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM parse_jobs
        WHERE status IN ('pending', 'processing')
        GROUP BY document_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'duplicate active parse jobs exist; resolve them before applying uq_parse_jobs_active_document';
    END IF;
END
$$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_parse_jobs_active_document
    ON parse_jobs(document_id)
    WHERE status IN ('pending', 'processing');

COMMIT;
