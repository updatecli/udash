-- Only the index is dropped: emptying pipeline_result and pipeline_name again would
-- destroy data rather than restore the previous state.
BEGIN;

DROP INDEX IF EXISTS idx_pipelinereports_updated_at_pipeline_result;

COMMIT;
