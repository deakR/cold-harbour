-- +goose Up
-- +goose StatementBegin
ALTER TABLE job_accepts
    ADD COLUMN source TEXT NOT NULL DEFAULT 'cold-harbour';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE job_accepts DROP COLUMN source;
-- +goose StatementEnd
