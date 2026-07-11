ALTER TABLE download_tokens ADD COLUMN purpose TEXT NOT NULL DEFAULT 'read';
CREATE INDEX IF NOT EXISTS idx_download_tokens_note ON download_tokens(note_id);