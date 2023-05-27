BEGIN;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE IF NOT EXISTS pipelineReports(
   id uuid DEFAULT uuid_generate_v4 (),
   data JSON NOT NULL,
   created_at TIMESTAMP,
   updated_at TIMESTAMP
);
ALTER TABLE pipelineReports ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE pipelineReports ALTER COLUMN updated_at SET DEFAULT now();
COMMIT;
