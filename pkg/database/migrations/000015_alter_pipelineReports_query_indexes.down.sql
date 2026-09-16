BEGIN;

DROP INDEX IF EXISTS idx_config_targets_config;
DROP INDEX IF EXISTS idx_config_conditions_config;
DROP INDEX IF EXISTS idx_config_sources_config;

CREATE INDEX IF NOT EXISTS idx_pipelinereports_distinct
ON pipelinereports ((data ->> 'Name'), updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_pipelinereports_data_result
ON pipelinereports ((data ->> 'Result'));

CREATE INDEX IF NOT EXISTS idx_pipelinereports_data_name
ON pipelinereports ((data ->> 'Name'));

CREATE INDEX IF NOT EXISTS idx_pipelinereports_data_jsonb
ON pipelinereports USING gin (data jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_pipelinereports_updated_at_result_open_action
ON pipelineReports (
    updated_at,
    pipeline_result,
    (jsonb_path_exists(data, '$.Actions.*.actionUrl'))
);

DROP INDEX IF EXISTS idx_pipelinereports_open_action;
DROP INDEX IF EXISTS idx_pipelinereports_pipeline_id_updated_at;
DROP INDEX IF EXISTS idx_pipelinereports_id;

COMMIT;
