BEGIN;
CREATE EXTENSION IF NOT EXISTS hstore;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create the config_source table
CREATE TABLE IF NOT EXISTS config_sources (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    kind text NOT NULL,
    config jsonb NOT NULL,
    created_at timestamp,
    updated_at timestamp
);
ALTER TABLE config_sources ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE config_sources ALTER COLUMN updated_at SET DEFAULT now();

-- Create the config_condition table
CREATE TABLE IF NOT EXISTS config_conditions (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    kind text NOT NULL,
    config jsonb NOT NULL,
    created_at timestamp,
    updated_at timestamp
);
ALTER TABLE config_conditions ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE config_conditions ALTER COLUMN updated_at SET DEFAULT now();

-- Create the config_target table
CREATE TABLE IF NOT EXISTS config_targets (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    kind text NOT NULL,
    config jsonb NOT NULL,
    created_at timestamp,
    updated_at timestamp
);
ALTER TABLE config_targets ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE config_targets ALTER COLUMN updated_at SET DEFAULT now();

-- Add the config_source_id, config_condition_id, and config_target_id columns
-- to the pipelineReports table
ALTER TABLE pipelinereports
ADD COLUMN IF NOT EXISTS config_source_ids hstore,
ADD COLUMN IF NOT EXISTS config_condition_ids hstore,
ADD COLUMN IF NOT EXISTS config_target_ids hstore;

CREATE INDEX IF NOT EXISTS idx_pipelinereports_config_source_id
ON pipelinereports USING gin (config_source_ids);

CREATE INDEX IF NOT EXISTS idx_pipelinereports_config_condition_id
ON pipelinereports USING gin (config_condition_ids);

CREATE INDEX IF NOT EXISTS idx_pipelinereports_config_target_id
ON pipelinereports USING gin (config_target_ids);

COMMIT;
