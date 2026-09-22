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
    tenant_id UUID NOT NULL REFERENCES tenants(id)
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

INSERT INTO tenants (id, name) VALUES
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'tenant-a'),
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 'tenant-b');

INSERT INTO api_keys (tenant_id, key_hash) VALUES
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'dae4ab37f024ffc97414b5645b594b43ec7a4ab86cadaf5ca9b73a0224152186'),
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     'b1fc4a424e7ee83b62c96e2215e7f94d6c19540a5a35ce5ef4e6cb5a96f49775');

