ALTER TABLE pipelineReports
    ALTER COLUMN data
    SET DATA TYPE JSONB
    USING data::JSONB;

CREATE INDEX idx_pipelinereports_data_jsonb
ON pipelinereports
USING gin (data jsonb_path_ops);

CREATE INDEX idx_pipelinereports_updated_at
ON pipelinereports (updated_at);

CREATE INDEX idx_pipelinereports_data_name
ON pipelinereports ((data ->> 'Name'));

CREATE INDEX idx_pipelinereports_data_result
ON pipelinereports ((data ->> 'Result'));

CREATE INDEX idx_pipelinereports_distinct
ON pipelinereports (
    (data ->> 'Name'), updated_at DESC
);
