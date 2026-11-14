CREATE SCHEMA shops;
CREATE TABLE shops.stores (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
-- Separate membership identity keeps User and Store independent. These unique
-- constraints deliberately enforce the current one-store/one-manager policy.
CREATE TABLE shops.memberships (
    user_id text PRIMARY KEY REFERENCES identity.users(id),
    store_id text NOT NULL UNIQUE REFERENCES shops.stores(id),
    role text NOT NULL DEFAULT 'manager' CHECK (role = 'manager')
);
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version = 4;
ALTER TABLE infrastructure.schema_version ADD CHECK (version = 4);
