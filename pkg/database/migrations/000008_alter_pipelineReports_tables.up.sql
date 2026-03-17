-- This migration will alter the pipelineReports table in the database, which will store the pipeline reports associated with the database, by adding a new column called label_ids, which will store the list of unique identifiers of the labels associated with the database.
BEGIN;
ALTER TABLE pipelineReports
    ADD COLUMN IF NOT EXISTS label_ids UUID[] DEFAULT ARRAY[]::UUID[];

-- Trigger function: when a new pipeline report is inserted,
-- mark all referenced labels with the report timestamp.
CREATE OR REPLACE FUNCTION sync_labels_last_pipeline_report_at()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE labels
    SET last_pipeline_report_at = NEW.created_at
    WHERE id = ANY(NEW.label_ids);

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger definition
DROP TRIGGER IF EXISTS trg_sync_labels_last_pipeline_report_at ON pipelineReports;
CREATE TRIGGER trg_sync_labels_last_pipeline_report_at
AFTER INSERT ON pipelineReports
FOR EACH ROW
WHEN (array_length(NEW.label_ids, 1) IS NOT NULL)
EXECUTE FUNCTION sync_labels_last_pipeline_report_at();
COMMIT;