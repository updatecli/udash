BEGIN;
DROP INDEX IF EXISTS idx_pipelinereports_label_ids;
DROP INDEX IF EXISTS idx_config_sources_updated_at;
DROP INDEX IF EXISTS idx_config_conditions_updated_at;
DROP INDEX IF EXISTS idx_config_targets_updated_at;
COMMIT;
