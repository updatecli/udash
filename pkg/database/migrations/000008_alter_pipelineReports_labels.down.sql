DROP TRIGGER IF EXISTS trg_sync_labels_last_pipeline_report_at ON pipelineReports;
DROP FUNCTION IF EXISTS sync_labels_last_pipeline_report_at();

ALTER TABLE pipelineReports
    DROP COLUMN IF EXISTS label_ids;