ALTER TABLE pipelineReports 
    DROP COLUMN IF EXISTS pipeline_id,
    DROP COLUMN IF EXISTS pipeline_name,
    DROP COLUMN IF EXISTS pipeline_result,
    DROP COLUMN IF EXISTS target_db_scm_ids;

DROP INDEX IF EXISTS idx_pipelinereports_target_db_scm_ids;
