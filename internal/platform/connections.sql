CREATE SCHEMA instagram;
CREATE TABLE instagram.connections (
    id text PRIMARY KEY,
    store_id text NOT NULL UNIQUE REFERENCES shops.stores(id),
    provider text NOT NULL CHECK (provider = 'fake'),
    account_id text NOT NULL,
    page_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('connected','disconnected')),
    receiving_since timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX connections_active_page ON instagram.connections(provider,page_id) WHERE status='connected';
CREATE TABLE instagram.connection_attempts (
    state_hash text PRIMARY KEY,
    store_id text NOT NULL REFERENCES shops.stores(id),
    user_id text NOT NULL REFERENCES identity.users(id),
    expires_at timestamptz NOT NULL,
    completed boolean NOT NULL DEFAULT false
);
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version=5;
ALTER TABLE infrastructure.schema_version ADD CHECK (version=5);
