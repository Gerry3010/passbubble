CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Users
CREATE TABLE IF NOT EXISTS users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email             VARCHAR(255) UNIQUE NOT NULL,
    name              VARCHAR(255) NOT NULL,
    role              VARCHAR(50)  NOT NULL DEFAULT 'user',
    status            VARCHAR(50)  NOT NULL DEFAULT 'pending',
    password_hash     TEXT         NOT NULL,
    invited_by        UUID         REFERENCES users(id),

    -- Public keys (plaintext, needed for sharing)
    pub_x25519        TEXT         NOT NULL, -- base64-encoded 32 bytes
    pub_mlkem768      TEXT         NOT NULL, -- base64-encoded 1184 bytes

    -- Private keys encrypted with master key (Argon2id → AES-256-GCM)
    enc_priv_x25519   TEXT         NOT NULL,
    enc_priv_mlkem768 TEXT         NOT NULL,

    -- Argon2id KDF parameters
    kdf_salt   TEXT    NOT NULL,
    kdf_time   INTEGER NOT NULL DEFAULT 3,
    kdf_memory INTEGER NOT NULL DEFAULT 65536,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Invitations (admin-only user creation)
CREATE TABLE IF NOT EXISTS invitations (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email       VARCHAR(255) NOT NULL,
    token       VARCHAR(255) NOT NULL UNIQUE,
    invited_by  UUID        REFERENCES users(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Sessions (refresh token tracking)
CREATE TABLE IF NOT EXISTS sessions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  VARCHAR(255) NOT NULL UNIQUE,
    device_name VARCHAR(255),
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- Folders (hierarchical)
CREATE TABLE IF NOT EXISTS folders (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    parent_id  UUID        REFERENCES folders(id),
    owner_id   UUID        NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_folders_owner ON folders(owner_id);
CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id);

-- Folder permissions (sharing)
CREATE TABLE IF NOT EXISTS folder_permissions (
    folder_id  UUID        NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    permission VARCHAR(50) NOT NULL, -- 'read', 'write', 'owner'
    granted_by UUID        REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (folder_id, user_id)
);

-- Password entries (encrypted E2E)
CREATE TABLE IF NOT EXISTS entries (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    folder_id      UUID        REFERENCES folders(id),
    owner_id       UUID        NOT NULL REFERENCES users(id),
    type           VARCHAR(50) NOT NULL, -- 'password','totp','note','api-key','ssh-key','certificate'
    name           VARCHAR(255) NOT NULL,
    url            VARCHAR(1024),

    -- E2E encrypted payload: JSON { username, password, totp_secret, notes, ... }
    -- Encrypted with a random data_key via AES-256-GCM
    encrypted_data BYTEA       NOT NULL,
    data_nonce     BYTEA       NOT NULL, -- 12-byte AES-GCM nonce

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_entries_owner   ON entries(owner_id);
CREATE INDEX IF NOT EXISTS idx_entries_folder  ON entries(folder_id);
CREATE INDEX IF NOT EXISTS idx_entries_name    ON entries USING gin(to_tsvector('simple', name));

-- Per-user encrypted data key (enables E2E sharing)
-- The data_key is encrypted for each authorized user via Hybrid KEM (X25519 + ML-KEM-768)
CREATE TABLE IF NOT EXISTS entry_keys (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_id      UUID NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    encrypted_key BYTEA NOT NULL, -- hybrid-KEM-encrypted data_key
    UNIQUE (entry_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_entry_keys_user ON entry_keys(user_id);

-- Entry permissions (sharing)
CREATE TABLE IF NOT EXISTS entry_permissions (
    entry_id   UUID        NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    permission VARCHAR(50) NOT NULL, -- 'read', 'write', 'owner'
    granted_by UUID        REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (entry_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_entry_permissions_user ON entry_permissions(user_id);

-- Backups metadata (actual encrypted backup files stored on disk)
CREATE TABLE IF NOT EXISTS backups (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    filename   VARCHAR(500) NOT NULL UNIQUE,
    size       BIGINT      NOT NULL DEFAULT 0,
    created_by UUID        REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
