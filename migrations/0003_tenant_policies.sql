-- +goose Up
-- +goose StatementBegin
CREATE TABLE tenant_policies (
    tenant_id  UUID PRIMARY KEY REFERENCES tenants(id),
    version    INTEGER NOT NULL,
    document   JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE tenant_policies;
-- +goose StatementEnd
