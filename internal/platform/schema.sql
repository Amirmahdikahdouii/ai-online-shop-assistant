CREATE SCHEMA infrastructure;
CREATE TABLE infrastructure.schema_version (version integer PRIMARY KEY CHECK (version = 1));
INSERT INTO infrastructure.schema_version VALUES (1);
CREATE TABLE infrastructure.health_probe (
  id uuid PRIMARY KEY,
  checks bigint NOT NULL DEFAULT 0
);
CREATE TABLE infrastructure.worker_heartbeat (
  id text PRIMARY KEY CHECK (id = 'worker'),
  seen_at timestamptz NOT NULL
);
