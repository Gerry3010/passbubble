ALTER TABLE users
    ADD COLUMN IF NOT EXISTS totp_secret  TEXT,
    ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS totp_recovery_tokens (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token      VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ  NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_totp_recovery_user ON totp_recovery_tokens(user_id);
