CREATE SCHEMA processing;
CREATE TABLE processing.events (
    id text PRIMARY KEY,
    store_id text NOT NULL REFERENCES shops.stores(id),
    connection_id text NOT NULL REFERENCES instagram.connections(id),
    external_id text NOT NULL,
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    speaker text NOT NULL CHECK (speaker IN ('customer','shop')),
    scenario text NOT NULL CHECK (scenario IN ('message','failure','old','shop')),
    UNIQUE(connection_id, external_id)
);
CREATE TABLE processing.jobs (
    event_id text PRIMARY KEY REFERENCES processing.events(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','succeeded','failed')),
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    lease_until timestamptz,
    lease_token text,
    reason text NOT NULL DEFAULT 'queued' CHECK (reason IN ('queued','working','processed','sample_failure'))
);
CREATE INDEX jobs_claimable ON processing.jobs(available_at, event_id) WHERE status IN ('pending','processing');
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version=6;
ALTER TABLE infrastructure.schema_version ADD CHECK (version=6);
