BEGIN;
DROP TRIGGER IF EXISTS trg_sync_scms_last_pipeline_report_at ON pipelineReports;
DROP FUNCTION IF EXISTS sync_scms_last_pipeline_report_at();

ALTER TABLE scms
    DROP COLUMN IF EXISTS last_pipeline_report_at;
COMMIT;