CREATE TABLE IF NOT EXISTS share_links (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    token             VARCHAR(64) NOT NULL UNIQUE,
    owner_id          UUID        NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    entry_id          UUID        REFERENCES entries(id) ON DELETE CASCADE,
    folder_id         UUID        REFERENCES folders(id) ON DELETE CASCADE,
    encrypted_payload BYTEA       NOT NULL,
    payload_nonce     BYTEA       NOT NULL,
    password_salt     BYTEA,
    password_hash     BYTEA,
    max_views         INT,
    view_count        INT         NOT NULL DEFAULT 0,
    expires_at        TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at        TIMESTAMPTZ,
    CHECK ((entry_id IS NOT NULL) <> (folder_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_share_links_token ON share_links (token);
CREATE INDEX IF NOT EXISTS idx_share_links_owner ON share_links (owner_id);
