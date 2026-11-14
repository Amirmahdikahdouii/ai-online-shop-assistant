CREATE SCHEMA catalog;
CREATE TABLE catalog.products (
 id text PRIMARY KEY,
 store_id text NOT NULL REFERENCES shops.stores(id),
 name text NOT NULL,
 status text NOT NULL CHECK (status IN ('draft','approved','archived')),
 revision integer NOT NULL CHECK (revision > 0),
 variants jsonb NOT NULL CHECK (jsonb_typeof(variants)='array'),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX products_store_page ON catalog.products(store_id,id);
-- Reservations survive edits and archiving. A code never changes identity.
CREATE TABLE catalog.codes (
 store_id text NOT NULL REFERENCES shops.stores(id),
 code text NOT NULL,
 product_id text NOT NULL REFERENCES catalog.products(id),
 variant_id text NOT NULL,
 PRIMARY KEY(store_id,code)
);
CREATE TABLE catalog.history (
 product_id text NOT NULL REFERENCES catalog.products(id),
 revision integer NOT NULL,
 actor_id text NOT NULL REFERENCES identity.users(id),
 changed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 snapshot jsonb NOT NULL,
 PRIMARY KEY(product_id,revision)
);
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version=7;
ALTER TABLE infrastructure.schema_version ADD CHECK (version=7);
