CREATE TABLE catalog.extraction_jobs (
 selection_id text PRIMARY KEY REFERENCES catalog.post_selections(id) ON DELETE CASCADE,
 attempts integer NOT NULL DEFAULT 0,
 available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 lease_until timestamptz,
 lease_token text,
 reason text NOT NULL DEFAULT 'queued'
);
ALTER TABLE catalog.post_selections ADD COLUMN product_id text UNIQUE REFERENCES catalog.products(id);
ALTER TABLE catalog.post_selections ADD COLUMN proposal jsonb;
-- Upgrade existing selections to durable work too.
INSERT INTO catalog.extraction_jobs(selection_id) SELECT id FROM catalog.post_selections WHERE status='pending';
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version=9;
ALTER TABLE infrastructure.schema_version ADD CHECK (version=9);
