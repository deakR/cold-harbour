CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    job_type VARCHAR(64) NOT NULL,
    final_state VARCHAR(16) NOT NULL,
    output_checksum VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL
);
