-- This migration will create the labels table in the database, which will store the labels associated with the database.
BEGIN;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE IF NOT EXISTS labels(
   id uuid                 DEFAULT uuid_generate_v4 (),
   key                     VARCHAR NOT NULL,
   value                   VARCHAR NOT NULL,
   created_at              TIMESTAMP,
   updated_at              TIMESTAMP,
   last_pipeline_report_at TIMESTAMP,
   CONSTRAINT labels_key_value_unique UNIQUE (key, value)
);
ALTER TABLE labels ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE labels ALTER COLUMN updated_at SET DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_labels_key ON labels (key);
CREATE INDEX IF NOT EXISTS idx_labels_value ON labels (value);

COMMIT;