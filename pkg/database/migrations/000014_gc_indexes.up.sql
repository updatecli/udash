-- Indexes the garbage collector relies on.
--
-- It looks up, for every label, whether a report still references it, which
-- label_ids had no index for. It also deletes the stale resource configs in
-- batches, each of which looks for the next rows by updated_at.
BEGIN;
CREATE INDEX IF NOT EXISTS idx_pipelinereports_label_ids
ON pipelineReports USING gin (label_ids);

CREATE INDEX IF NOT EXISTS idx_config_sources_updated_at
ON config_sources (updated_at);

CREATE INDEX IF NOT EXISTS idx_config_conditions_updated_at
ON config_conditions (updated_at);

CREATE INDEX IF NOT EXISTS idx_config_targets_updated_at
ON config_targets (updated_at);
COMMIT;
