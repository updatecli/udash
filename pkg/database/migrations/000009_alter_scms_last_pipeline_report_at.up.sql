-- This migration adds a timestamp column to the scms table and keeps it
-- synchronized with the latest pipeline report creation time for linked SCMs.
BEGIN;
ALTER TABLE scms
    ADD COLUMN IF NOT EXISTS last_pipeline_report_at TIMESTAMP;

CREATE OR REPLACE FUNCTION sync_scms_last_pipeline_report_at()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE scms
    SET last_pipeline_report_at = NEW.created_at
    WHERE id = ANY(NEW.target_db_scm_ids);

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_scms_last_pipeline_report_at ON pipelineReports;
CREATE TRIGGER trg_sync_scms_last_pipeline_report_at
AFTER INSERT ON pipelineReports
FOR EACH ROW
WHEN (array_length(NEW.target_db_scm_ids, 1) IS NOT NULL)
EXECUTE FUNCTION sync_scms_last_pipeline_report_at();
COMMIT;