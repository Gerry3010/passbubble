CREATE TABLE IF NOT EXISTS jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type            VARCHAR(50) NOT NULL,
    format          VARCHAR(50) NOT NULL,
    status          VARCHAR(50) NOT NULL DEFAULT 'running',
    dup_strategy    VARCHAR(20) NOT NULL DEFAULT 'skip',
    total_items     INT NOT NULL DEFAULT 0,
    processed_items INT NOT NULL DEFAULT 0,
    created_items   INT NOT NULL DEFAULT 0,
    updated_items   INT NOT NULL DEFAULT 0,
    skipped_items   INT NOT NULL DEFAULT 0,
    failed_items    INT NOT NULL DEFAULT 0,
    error_message   TEXT,
    client_name     VARCHAR(100),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_jobs_user ON jobs(user_id);
CREATE INDEX IF NOT EXISTS idx_jobs_user_status ON jobs(user_id, status);
