-- This migration will add the pipeline_id, pipeline_name, pipeline_result, and target_db_scm_ids columns to the pipelineReports table.

ALTER TABLE pipelineReports
    ADD COLUMN IF NOT EXISTS pipeline_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pipeline_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pipeline_result TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS target_db_scm_ids UUID[] NOT NULL DEFAULT ARRAY[]::UUID[];

CREATE INDEX idx_pipelinereports_target_db_scm_ids ON pipelinereports USING gin (target_db_scm_ids);

-- This query will update the pipeline_id, pipeline_name, and pipeline_result columns in the pipelinereports table

UPDATE pipelineReports
SET 
    pipeline_id = COALESCE(NULLIF(TRIM(data ->> 'ID'), ''), pipeline_id),
    pipeline_result = COALESCE(NULLIF(TRIM(data ->> 'result'), ''), pipeline_result),
    pipeline_name = COALESCE(NULLIF(TRIM(data ->> 'name'), ''), pipeline_name)
WHERE 
    (pipeline_id IS NULL OR TRIM(pipeline_id) = '') 
    OR (pipeline_result IS NULL OR TRIM(pipeline_result) = '') 
    OR (pipeline_name IS NULL OR TRIM(pipeline_name) = '');

-- This query will update the target_db_scm_ids column in the pipelineReports table
-- with the ids of the scms that match the scm url and branch target in the data column.

UPDATE pipelineReports as pr
SET "target_db_scm_ids" = (
    SELECT array_agg(s.id) 
    FROM "scms" AS s
    WHERE TRIM(LOWER(s.url)) IN (
        SELECT TRIM(LOWER(value->'Scm'->>'URL'))
        FROM LATERAL jsonb_path_query(pr.data, '$.Targets[*].* ? (@.Scm.URL != "" && @.Scm.Branch.Target != "")') AS value
    )
    AND TRIM(LOWER(s.branch)) IN (
        SELECT TRIM(LOWER(value->'Scm'->'Branch'->>'Target'))
        FROM LATERAL jsonb_path_query(pr.data, '$.Targets[*].* ? (@.Scm.URL != "" && @.Scm.Branch.Target != "")') AS value
    )
)
WHERE 
    jsonb_path_exists(pr.data, '$.Targets[*].* ? (@.Scm.URL != "" && @.Scm.Branch.Target != "")')
    AND (pr.target_db_scm_ids IS NULL OR cardinality(pr.target_db_scm_ids) = 0);