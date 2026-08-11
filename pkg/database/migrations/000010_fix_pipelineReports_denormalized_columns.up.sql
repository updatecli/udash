-- Migration 000004 backfilled pipeline_result and pipeline_name from "data ->> 'result'"
-- and "data ->> 'name'", but a marshalled report stores those keys as "Result" and "Name".
-- jsonb keys are case sensitive so NULLIF(TRIM(...), '') always evaluated to NULL, the
-- COALESCE fell back to the column's own default and the backfill did nothing. Only
-- pipeline_id used the right casing, which is why it is the only one of the three that is
-- queried today. Every report inserted before 000004 therefore still has an empty
-- pipeline_result, which the reports summary would report as an unknown result.
BEGIN;

UPDATE pipelineReports
SET
    pipeline_result = COALESCE(NULLIF(TRIM(data ->> 'Result'), ''), pipeline_result),
    pipeline_name   = COALESCE(NULLIF(TRIM(data ->> 'Name'), ''), pipeline_name)
WHERE
    TRIM(pipeline_result) = ''
    OR TRIM(pipeline_name) = '';

-- The reports summary groups the reports of a time range per result. idx_pipelinereports_updated_at
-- already serves the range predicate but the result still has to be fetched from the heap
-- row by row, so a composite index is what makes the aggregation an index only scan.
CREATE INDEX IF NOT EXISTS idx_pipelinereports_updated_at_pipeline_result
ON pipelineReports (updated_at, pipeline_result);

COMMIT;
