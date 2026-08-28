BEGIN;

DROP INDEX IF EXISTS idx_pipelinereports_created_by_subject;
ALTER TABLE pipelineReports DROP COLUMN IF EXISTS created_by_token_id;
ALTER TABLE pipelineReports DROP COLUMN IF EXISTS created_by_subject;

COMMIT;
