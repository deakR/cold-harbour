-- +goose Up
-- +goose StatementBegin
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    key_hash VARCHAR(128) NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX api_keys_key_hash_uidx ON api_keys (key_hash);

CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    job_type VARCHAR(64) NOT NULL,
    final_state VARCHAR(16) NOT NULL,
    output_checksum VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    output_signature BYTEA,
    signing_key_id TEXT
);

CREATE TABLE outputs (
    redis_job_id TEXT PRIMARY KEY,
    final_state VARCHAR(16) NOT NULL,
    body TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

CREATE TABLE job_accepts (
    redis_job_id TEXT PRIMARY KEY,
    accepted_at TIMESTAMPTZ NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

CREATE TABLE job_keys (
    job_id UUID PRIMARY KEY,
    key_material BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    destroyed_at TIMESTAMPTZ
);

CREATE TABLE purge_receipts (
    job_id UUID PRIMARY KEY,
    redis_job_id TEXT NOT NULL,
    purged_at TIMESTAMPTZ NOT NULL,
    signature BYTEA NOT NULL,
    signing_key_id TEXT NOT NULL
);

CREATE TABLE delivery_links (
    token_hash VARCHAR(64) PRIMARY KEY,
    redis_job_id TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    expires_at TIMESTAMPTZ NOT NULL,
    max_views INT NOT NULL,
    view_count INT NOT NULL DEFAULT 0
);

CREATE TABLE delivery_link_accesses (
    id BIGSERIAL PRIMARY KEY,
    token_hash VARCHAR(64) NOT NULL,
    accessed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS delivery_link_accesses, delivery_links, purge_receipts, job_keys, job_accepts, outputs, jobs, api_keys, tenants;
-- +goose StatementEnd
