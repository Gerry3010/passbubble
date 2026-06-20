-- URL match patterns for autofill (Psono-style urlfilter). Plaintext metadata,
-- matched in the extension background without decrypting the entry. Nullable;
-- NULL/empty array means "fall back to matching on the url field".
ALTER TABLE entries ADD COLUMN IF NOT EXISTS match_patterns TEXT[];
