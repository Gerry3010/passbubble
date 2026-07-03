-- Trash (soft delete), favorites, and entry version history.
--
-- entry_versions snapshots the previous encrypted blob whenever an entry's
-- data is overwritten. Every update re-wraps a FRESH per-entry data key, so a
-- version's blob is only readable with the entry_keys rows that existed at
-- snapshot time — they are copied into entry_version_keys (the server only
-- copies ciphertext it already stores; E2E is preserved).

ALTER TABLE entries ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE entries ADD COLUMN favorite BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX idx_entries_deleted ON entries(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE TABLE entry_versions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_id       UUID NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    name           VARCHAR(255) NOT NULL,
    url            VARCHAR(1024),
    encrypted_data BYTEA NOT NULL,
    data_nonce     BYTEA NOT NULL,
    edited_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_entry_versions_entry ON entry_versions(entry_id, created_at DESC);

CREATE TABLE entry_version_keys (
    version_id    UUID  NOT NULL REFERENCES entry_versions(id) ON DELETE CASCADE,
    user_id       UUID  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    encrypted_key BYTEA NOT NULL,
    PRIMARY KEY (version_id, user_id)
);
