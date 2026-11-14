CREATE TABLE identity.sessions (
 token_hash text PRIMARY KEY,
 user_id text NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
 csrf_token text NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expiry ON identity.sessions(expires_at);
CREATE TABLE identity.login_attempts (
 bucket text PRIMARY KEY,
 attempts integer NOT NULL,
 expires_at timestamptz NOT NULL
);
ALTER TABLE infrastructure.schema_version DROP CONSTRAINT schema_version_version_check;
UPDATE infrastructure.schema_version SET version=3;
ALTER TABLE infrastructure.schema_version ADD CHECK (version=3);
