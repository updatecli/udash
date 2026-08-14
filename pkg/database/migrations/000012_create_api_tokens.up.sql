BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS api_tokens(
   id           uuid    DEFAULT uuid_generate_v4 (),
   name         VARCHAR NOT NULL,
   -- Only the sha256 of the token is kept: the token itself is shown once, when
   -- it is created, and can never be recovered from here.
   token_hash   BYTEA   NOT NULL,
   -- subject is the identity provider subject which created the token.
   subject      VARCHAR NOT NULL,
   -- permission is what the creator could do when the token was issued. It bounds
   -- the token when the current permission cannot be looked up.
   permission   VARCHAR NOT NULL,
   scopes       TEXT[]  NOT NULL DEFAULT '{}',
   -- These are TIMESTAMPTZ, unlike the older tables: expiry is compared against
   -- the current instant, and a TIMESTAMP drops the offset on the way back out,
   -- which moves a token's expiry by the server's UTC offset.
   created_at   TIMESTAMPTZ,
   last_used_at TIMESTAMPTZ,
   -- A NULL expiry means the token never expires, which is the point of it.
   expires_at   TIMESTAMPTZ,
   CONSTRAINT api_tokens_pkey PRIMARY KEY (id),
   CONSTRAINT api_tokens_token_hash_unique UNIQUE (token_hash)
);

ALTER TABLE api_tokens ALTER COLUMN created_at SET DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_api_tokens_subject ON api_tokens (subject);

COMMIT;
