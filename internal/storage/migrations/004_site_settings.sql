CREATE TABLE IF NOT EXISTS site_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  hero_title TEXT NOT NULL DEFAULT 'TimeNotes Blog',
  hero_subtitle TEXT NOT NULL DEFAULT '浏览公开手账本 · 点赞 · 评论',
  background_mode TEXT NOT NULL DEFAULT 'none',
  background_path TEXT NOT NULL DEFAULT '',
  background_url TEXT NOT NULL DEFAULT '',
  focus_x REAL NOT NULL DEFAULT 50,
  focus_y REAL NOT NULL DEFAULT 40,
  overlay_color TEXT NOT NULL DEFAULT '#0b0d12',
  overlay_opacity REAL NOT NULL DEFAULT 0.45,
  updated_at TEXT NOT NULL
);

INSERT OR IGNORE INTO site_settings (
  id, hero_title, hero_subtitle, background_mode, background_path, background_url,
  focus_x, focus_y, overlay_color, overlay_opacity, updated_at
) VALUES (
  1, 'TimeNotes Blog', '浏览公开手账本 · 点赞 · 评论', 'none', '', '',
  50, 40, '#0b0d12', 0.45, datetime('now')
);
