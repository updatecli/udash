BEGIN;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE IF NOT EXISTS scms(
   id uuid DEFAULT uuid_generate_v4 (),
   branch VARCHAR NOT NULL,
   url VARCHAR NOT NULL,
   created_at TIMESTAMP,
   updated_at TIMESTAMP
);
ALTER TABLE scms ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE scms ALTER COLUMN updated_at SET DEFAULT now();
COMMIT;
