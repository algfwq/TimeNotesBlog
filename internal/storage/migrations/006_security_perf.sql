ALTER TABLE users ADD COLUMN credentials_version INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_visits_ip_hash ON visits(ip_hash);
CREATE INDEX IF NOT EXISTS idx_visits_created_asc ON visits(created_at ASC);
