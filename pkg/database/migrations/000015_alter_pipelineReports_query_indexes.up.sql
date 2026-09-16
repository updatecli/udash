-- Indexes letting the report queries find what they need without reading report payloads.
--
-- A report payload weighs several kilobytes stored compressed out of line, so a query
-- reading one per row slows down with every report it looks at. Measured over a year of
-- reports published at 2000 a day, that is what made opening a report, the results
-- summary and the dashboard slow.
BEGIN;

-- Opening a report looks it up by id, then counts and fetches the latest report of its
-- pipeline. None of those columns was indexed, so each lookup read the whole table.
CREATE INDEX IF NOT EXISTS idx_pipelinereports_id
ON pipelineReports (id);

CREATE INDEX IF NOT EXISTS idx_pipelinereports_pipeline_id_updated_at
ON pipelineReports (pipeline_id, updated_at);

-- Postgres cannot return an indexed expression from an index only scan, so the index of
-- migration 000011 still left the summary reading every payload to evaluate it. The
-- predicate of a partial index never has to be returned though: the summary counts every
-- report through idx_pipelinereports_updated_at_pipeline_result, then the ones carrying
-- an open action through this one, both without touching a payload.
--
-- The predicate must stay byte for byte openActionSQLExpr, otherwise the summary keeps
-- returning the right counts while silently reading every payload again.
CREATE INDEX IF NOT EXISTS idx_pipelinereports_open_action
ON pipelineReports (updated_at, pipeline_result)
WHERE jsonb_path_exists(data, '$.Actions.*.actionUrl');

DROP INDEX IF EXISTS idx_pipelinereports_updated_at_result_open_action;

-- No query filters on the payload itself, yet every published report had to update these,
-- the GIN index over the whole payload being the most expensive of all.
DROP INDEX IF EXISTS idx_pipelinereports_data_jsonb;
DROP INDEX IF EXISTS idx_pipelinereports_data_name;
DROP INDEX IF EXISTS idx_pipelinereports_data_result;
DROP INDEX IF EXISTS idx_pipelinereports_distinct;

-- Publishing a report looks up every config it references by containment.
CREATE INDEX IF NOT EXISTS idx_config_sources_config
ON config_sources USING gin (config jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_config_conditions_config
ON config_conditions USING gin (config jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_config_targets_config
ON config_targets USING gin (config jsonb_path_ops);

COMMIT;
