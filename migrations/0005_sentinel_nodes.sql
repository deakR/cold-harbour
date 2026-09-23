-- +goose Up
-- +goose StatementBegin
CREATE TABLE sentinel_nodes (
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    node_id        TEXT NOT NULL,
    last_seen      TIMESTAMPTZ NOT NULL,
    policy_version INTEGER NOT NULL,
    counts         JSONB NOT NULL DEFAULT '{}'::jsonb,
    held           BIGINT NOT NULL DEFAULT 0,
    dropped        BIGINT NOT NULL DEFAULT 0,
    fail_open      BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, node_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE sentinel_nodes;
-- +goose StatementEnd
