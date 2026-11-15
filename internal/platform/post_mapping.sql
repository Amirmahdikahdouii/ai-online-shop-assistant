ALTER TABLE catalog.post_selections ADD COLUMN mapping_revision integer NOT NULL DEFAULT 0 CHECK (mapping_revision >= 0);
ALTER TABLE catalog.post_selections ADD COLUMN mapping_not_product boolean NOT NULL DEFAULT false;
ALTER TABLE catalog.post_selections ADD COLUMN mapping_targets jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(mapping_targets)='array');
-- Immutable decisions double as a durable change feed for the later resume worker.
-- A consumer can checkpoint (selection_id, revision); no waiting work runs here.
CREATE TABLE catalog.post_mapping_changes (
 selection_id text NOT NULL REFERENCES catalog.post_selections(id) ON DELETE CASCADE,
 revision integer NOT NULL CHECK (revision > 0),
 actor_id text NOT NULL REFERENCES identity.users(id),
 source text NOT NULL CHECK (source IN ('manager')),
 changed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 snapshot jsonb NOT NULL,
 PRIMARY KEY(selection_id,revision)
);
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version=10;
ALTER TABLE infrastructure.schema_version ADD CHECK (version=10);
