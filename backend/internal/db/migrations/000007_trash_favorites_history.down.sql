DROP TABLE IF EXISTS entry_version_keys;
DROP TABLE IF EXISTS entry_versions;
DROP INDEX IF EXISTS idx_entries_deleted;
ALTER TABLE entries DROP COLUMN IF EXISTS favorite;
ALTER TABLE entries DROP COLUMN IF EXISTS deleted_at;
