-- +goose Up
-- +goose StatementBegin
CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    key_id       UUID NOT NULL REFERENCES api_keys(id),
    token_hash   BYTEA NOT NULL,
    csrf_token   TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ
);

CREATE UNIQUE INDEX sessions_token_hash_uidx ON sessions (token_hash);
CREATE INDEX sessions_tenant_idx ON sessions (tenant_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS sessions;
-- +goose StatementEnd
