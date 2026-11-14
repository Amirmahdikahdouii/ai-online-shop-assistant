CREATE TABLE catalog.post_selections (
    id text PRIMARY KEY,
    store_id text NOT NULL REFERENCES shops.stores(id),
    connection_id text NOT NULL REFERENCES instagram.connections(id),
    post_id text NOT NULL,
    caption text NOT NULL,
    published_at timestamptz NOT NULL,
    selected_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    actor_id text NOT NULL REFERENCES identity.users(id),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','completed','failed','blocked')),
    UNIQUE(store_id, connection_id, post_id)
);
CREATE TABLE catalog.post_selection_requests (
    store_id text NOT NULL REFERENCES shops.stores(id),
    user_id text NOT NULL REFERENCES identity.users(id),
    request_key text NOT NULL,
    body_hash text NOT NULL,
    result jsonb,
    PRIMARY KEY(store_id, user_id, request_key)
);
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version=8;
ALTER TABLE infrastructure.schema_version ADD CHECK (version=8);
