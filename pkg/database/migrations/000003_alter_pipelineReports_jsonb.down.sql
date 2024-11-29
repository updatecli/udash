ALTER TABLE pipelineReports
    ALTER COLUMN data
    SET DATA TYPE JSON
    USING data::JSON;

DROP INDEX idx_pipelinereports_data_jsonb;
DROP INDEX idx_pipelinereports_updated_at;
DROP INDEX idx_pipelinereports_data_name;
DROP INDEX idx_pipelinereports_data_result;
DROP INDEX idx_pipelinereports_distinct