ALTER TABLE users ADD COLUMN must_change_credentials INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notes ADD COLUMN cover_path TEXT NOT NULL DEFAULT '';
ALTER TABLE notes ADD COLUMN public_download INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_notes_owner ON notes(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_download_tokens_expires ON download_tokens(expires_at);
