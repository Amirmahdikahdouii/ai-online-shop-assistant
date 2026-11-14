CREATE SCHEMA identity;
CREATE TABLE identity.users (
  id text PRIMARY KEY,
  username text NOT NULL UNIQUE,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version = 2;
ALTER TABLE infrastructure.schema_version ADD CHECK (version = 2);
