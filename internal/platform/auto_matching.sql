ALTER TABLE catalog.post_selections ADD COLUMN match_status text NOT NULL DEFAULT 'none' CHECK (match_status IN ('none', 'matched', 'unresolved'));
ALTER TABLE catalog.post_selections ADD COLUMN match_evidence jsonb;
ALTER TABLE catalog.post_mapping_changes DROP CONSTRAINT post_mapping_changes_source_check;
ALTER TABLE catalog.post_mapping_changes ADD CHECK (source IN ('manager', 'system'));
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version=11;
ALTER TABLE infrastructure.schema_version ADD CHECK (version=11);
