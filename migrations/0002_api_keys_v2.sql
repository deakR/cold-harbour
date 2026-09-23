-- +goose Up
-- +goose StatementBegin
DELETE FROM api_keys WHERE key_hash IN (
    'dae4ab37f024ffc97414b5645b594b43ec7a4ab86cadaf5ca9b73a0224152186',
    'b1fc4a424e7ee83b62c96e2215e7f94d6c19540a5a35ce5ef4e6cb5a96f49775'
);

ALTER TABLE api_keys ADD COLUMN name TEXT;
ALTER TABLE api_keys ADD COLUMN prefix TEXT;
ALTER TABLE api_keys ADD COLUMN role TEXT;
ALTER TABLE api_keys ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE api_keys ALTER COLUMN key_hash TYPE BYTEA USING decode(key_hash, 'hex');

UPDATE api_keys SET name = 'legacy' WHERE name IS NULL;
UPDATE api_keys SET role = 'admin' WHERE role IS NULL;
UPDATE api_keys SET prefix = substr(replace(id::text, '-', ''), 1, 8) WHERE prefix IS NULL;

ALTER TABLE api_keys ALTER COLUMN name SET NOT NULL;
ALTER TABLE api_keys ALTER COLUMN prefix SET NOT NULL;
ALTER TABLE api_keys ALTER COLUMN role SET NOT NULL;

CREATE UNIQUE INDEX api_keys_prefix_uidx ON api_keys (prefix);

CREATE TABLE audit_ledger (
    tenant_id  UUID NOT NULL REFERENCES tenants(id),
    seq        BIGINT NOT NULL,
    prev_hash  BYTEA NOT NULL,
    entry_hash BYTEA NOT NULL,
    kind       TEXT NOT NULL,
    payload    JSONB NOT NULL,
    at         TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, seq)
);

CREATE TABLE ledger_heads (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id),
    seq       BIGINT NOT NULL,
    hash      BYTEA NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS ledger_heads, audit_ledger;
DROP INDEX IF EXISTS api_keys_prefix_uidx;
ALTER TABLE api_keys DROP COLUMN IF EXISTS name, DROP COLUMN IF EXISTS prefix, DROP COLUMN IF EXISTS role, DROP COLUMN IF EXISTS created_at;
-- +goose StatementEnd
