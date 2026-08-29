BEGIN;

-- Who published a report. Both are nullable: reports published before this
-- migration have no attribution, and neither do reports published against an
-- instance running without authentication.
ALTER TABLE pipelineReports ADD COLUMN IF NOT EXISTS created_by_subject VARCHAR;
ALTER TABLE pipelineReports ADD COLUMN IF NOT EXISTS created_by_token_id uuid;

CREATE INDEX IF NOT EXISTS idx_pipelinereports_created_by_subject ON pipelineReports (created_by_subject);

COMMIT;
