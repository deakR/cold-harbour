CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    job_type VARCHAR(64) NOT NULL,
    final_state VARCHAR(16) NOT NULL,
    output_checksum VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE outputs (
    redis_job_id TEXT PRIMARY KEY,
    final_state VARCHAR(16) NOT NULL,
    body TEXT NOT NULL
);

CREATE TABLE job_accepts (
    redis_job_id TEXT PRIMARY KEY,
    accepted_at TIMESTAMPTZ NOT NULL
);
