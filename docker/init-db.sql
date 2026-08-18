-- ColdHarbor Permanent Audit Database Initialization

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS audit_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    compartment_id VARCHAR(64) NOT NULL,
    owner_id VARCHAR(64) NOT NULL,
    context VARCHAR(16) NOT NULL CHECK (context IN ('INNIE', 'OUTIE', 'SYSTEM', 'ADMIN')),
    task_type VARCHAR(64) NOT NULL,
    final_state VARCHAR(32) NOT NULL CHECK (final_state IN ('COMPLETED', 'ARCHIVED', 'PURGED', 'FAILED')),
    checksum VARCHAR(64) NOT NULL,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_audit_compartment_id ON audit_records(compartment_id);
CREATE INDEX IF NOT EXISTS idx_audit_owner_id ON audit_records(owner_id);
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_records(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_context ON audit_records(context);
