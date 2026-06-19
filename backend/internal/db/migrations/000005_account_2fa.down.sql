DROP TABLE IF EXISTS totp_recovery_tokens;

ALTER TABLE users
    DROP COLUMN IF EXISTS totp_secret,
    DROP COLUMN IF EXISTS totp_enabled;
