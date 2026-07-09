package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"timenotesblog/internal/storage"
	"timenotesblog/internal/storage/migrations"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = filepath.Join("data", "blog.db")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-16000&_txlock=immediate&_foreign_keys=on"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS migration_versions (
		filename TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		var already string
		if err := s.db.QueryRowContext(ctx, `SELECT filename FROM migration_versions WHERE filename = ?`, name).Scan(&already); err == nil {
			continue
		}
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO migration_versions(filename, applied_at) VALUES(?, ?)`,
			name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) EnsureAdmin(ctx context.Context, username, passwordHash string) (bool, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id, username, password_hash, role, can_upload, created_at, updated_at)
		VALUES(?, ?, ?, 'admin', 1, ?, ?)`, "admin", username, passwordHash, now, now)
	return err == nil, err
}

func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, user storage.User) error {
	can := 0
	if user.CanUpload {
		can = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id, username, password_hash, role, can_upload, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash, user.Role, can, user.CreatedAt, user.UpdatedAt)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return storage.ErrConflict
	}
	return err
}

func scanUser(row interface{ Scan(dest ...any) error }) (*storage.User, error) {
	var u storage.User
	var can int
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &can, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	u.CanUpload = can == 1
	return &u, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*storage.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, role, can_upload, created_at, updated_at FROM users WHERE username = ?`, username)
	return scanUser(row)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*storage.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, role, can_upload, created_at, updated_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) ListUsers(ctx context.Context) ([]storage.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, password_hash, role, can_upload, created_at, updated_at FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storage.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, user storage.User) error {
	can := 0
	if user.CanUpload {
		can = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET username=?, password_hash=?, role=?, can_upload=?, updated_at=? WHERE id=?`,
		user.Username, user.PasswordHash, user.Role, can, user.UpdatedAt, user.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return storage.ErrConflict
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) UsernameExists(ctx context.Context, username string, excludeID string) (bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, username).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if excludeID != "" && id == excludeID {
		return false, nil
	}
	return true, nil
}

func scanNote(row interface{ Scan(dest ...any) error }, withOwner bool) (*storage.Note, error) {
	var n storage.Note
	var visible int
	var err error
	if withOwner {
		err = row.Scan(&n.ID, &n.OwnerUserID, &n.OwnerName, &n.Filename, &n.Title, &n.StoragePath, &n.SizeBytes, &n.SHA256, &visible, &n.LikeCount, &n.CommentCount, &n.CreatedAt, &n.UpdatedAt)
	} else {
		err = row.Scan(&n.ID, &n.OwnerUserID, &n.Filename, &n.Title, &n.StoragePath, &n.SizeBytes, &n.SHA256, &visible, &n.LikeCount, &n.CommentCount, &n.CreatedAt, &n.UpdatedAt)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	n.Visible = visible == 1
	return &n, nil
}

func (s *Store) CreateNote(ctx context.Context, note storage.Note) error {
	visible := 0
	if note.Visible {
		visible = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO notes(id, owner_user_id, filename, title, storage_path, size_bytes, sha256, visible, like_count, comment_count, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		note.ID, note.OwnerUserID, note.Filename, note.Title, note.StoragePath, note.SizeBytes, note.SHA256, visible, note.LikeCount, note.CommentCount, note.CreatedAt, note.UpdatedAt)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return storage.ErrConflict
	}
	return err
}

func (s *Store) UpdateNoteFile(ctx context.Context, note storage.Note) error {
	res, err := s.db.ExecContext(ctx, `UPDATE notes SET title=?, storage_path=?, size_bytes=?, sha256=?, updated_at=? WHERE id=?`,
		note.Title, note.StoragePath, note.SizeBytes, note.SHA256, note.UpdatedAt, note.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) GetNote(ctx context.Context, id string) (*storage.Note, error) {
	row := s.db.QueryRowContext(ctx, `SELECT n.id, n.owner_user_id, COALESCE(u.username,''), n.filename, n.title, n.storage_path, n.size_bytes, n.sha256, n.visible, n.like_count, n.comment_count, n.created_at, n.updated_at
		FROM notes n LEFT JOIN users u ON u.id = n.owner_user_id WHERE n.id = ?`, id)
	return scanNote(row, true)
}

func (s *Store) GetNoteByOwnerFilename(ctx context.Context, ownerID, filename string) (*storage.Note, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, owner_user_id, filename, title, storage_path, size_bytes, sha256, visible, like_count, comment_count, created_at, updated_at
		FROM notes WHERE owner_user_id = ? AND filename = ?`, ownerID, filename)
	return scanNote(row, false)
}

func (s *Store) listNotes(ctx context.Context, onlyVisible bool) ([]storage.Note, error) {
	q := `SELECT n.id, n.owner_user_id, COALESCE(u.username,''), n.filename, n.title, n.storage_path, n.size_bytes, n.sha256, n.visible, n.like_count, n.comment_count, n.created_at, n.updated_at
		FROM notes n LEFT JOIN users u ON u.id = n.owner_user_id`
	if onlyVisible {
		q += ` WHERE n.visible = 1`
	}
	q += ` ORDER BY n.updated_at DESC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storage.Note
	for rows.Next() {
		n, err := scanNote(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func (s *Store) ListVisibleNotes(ctx context.Context) ([]storage.Note, error) {
	return s.listNotes(ctx, true)
}

func (s *Store) ListAllNotes(ctx context.Context) ([]storage.Note, error) {
	return s.listNotes(ctx, false)
}

func (s *Store) SetNoteVisible(ctx context.Context, id string, visible bool) error {
	v := 0
	if visible {
		v = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE notes SET visible=?, updated_at=? WHERE id=?`, v, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteNote(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) AddLike(ctx context.Context, noteID, ipHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var exists string
	err = tx.QueryRowContext(ctx, `SELECT note_id FROM likes WHERE note_id=? AND ip_hash=?`, noteID, ipHash).Scan(&exists)
	if err == nil {
		return storage.ErrAlreadyLiked
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO likes(note_id, ip_hash, created_at) VALUES(?,?,?)`, noteID, ipHash, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET like_count = like_count + 1 WHERE id = ?`, noteID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) HasLiked(ctx context.Context, noteID, ipHash string) (bool, error) {
	var x string
	err := s.db.QueryRowContext(ctx, `SELECT note_id FROM likes WHERE note_id=? AND ip_hash=?`, noteID, ipHash).Scan(&x)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) AddComment(ctx context.Context, c storage.Comment) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO comments(id, note_id, nickname, email, github_url, content, created_at) VALUES(?,?,?,?,?,?,?)`,
		c.ID, c.NoteID, c.Nickname, c.Email, c.GitHubURL, c.Content, c.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET comment_count = comment_count + 1 WHERE id = ?`, c.NoteID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListComments(ctx context.Context, noteID string) ([]storage.Comment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, note_id, nickname, email, github_url, content, created_at FROM comments WHERE note_id = ? ORDER BY created_at DESC`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storage.Comment
	for rows.Next() {
		var c storage.Comment
		if err := rows.Scan(&c.ID, &c.NoteID, &c.Nickname, &c.Email, &c.GitHubURL, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetLoginFailures(ctx context.Context, ipHash string) (int, time.Time, error) {
	var count int
	var windowAt string
	err := s.db.QueryRowContext(ctx, `SELECT fail_count, window_at FROM login_failures WHERE ip_hash = ?`, ipHash).Scan(&count, &windowAt)
	if err == sql.ErrNoRows {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	t, _ := time.Parse(time.RFC3339Nano, windowAt)
	return count, t, nil
}

func (s *Store) BumpLoginFailure(ctx context.Context, ipHash string, now time.Time) (int, error) {
	count, windowAt, err := s.GetLoginFailures(ctx, ipHash)
	if err != nil {
		return 0, err
	}
	if windowAt.IsZero() || now.Sub(windowAt) > time.Hour {
		count = 1
		windowAt = now
	} else {
		count++
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO login_failures(ip_hash, fail_count, window_at) VALUES(?,?,?)
		ON CONFLICT(ip_hash) DO UPDATE SET fail_count=excluded.fail_count, window_at=excluded.window_at`,
		ipHash, count, windowAt.UTC().Format(time.RFC3339Nano))
	return count, err
}

func (s *Store) ResetLoginFailures(ctx context.Context, ipHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_failures WHERE ip_hash = ?`, ipHash)
	return err
}

func (s *Store) CreateDownloadToken(ctx context.Context, token, noteID string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO download_tokens(token, note_id, expires_at) VALUES(?,?,?)`,
		token, noteID, expiresAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetDownloadToken(ctx context.Context, token string) (string, time.Time, error) {
	var noteID, exp string
	err := s.db.QueryRowContext(ctx, `SELECT note_id, expires_at FROM download_tokens WHERE token = ?`, token).Scan(&noteID, &exp)
	if err == sql.ErrNoRows {
		return "", time.Time{}, storage.ErrNotFound
	}
	if err != nil {
		return "", time.Time{}, err
	}
	t, _ := time.Parse(time.RFC3339Nano, exp)
	return noteID, t, nil
}

func (s *Store) ConsumeDownloadToken(ctx context.Context, token string) (string, error) {
	noteID, exp, err := s.GetDownloadToken(ctx, token)
	if err != nil {
		return "", err
	}
	if time.Now().After(exp) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM download_tokens WHERE token = ?`, token)
		return "", storage.ErrNotFound
	}
	return noteID, nil
}

func (s *Store) GetGeoCache(ctx context.Context, ipHash string, maxAge time.Duration) (*storage.GeoInfo, error) {
	var info storage.GeoInfo
	var lat, lng sql.NullFloat64
	var fetched string
	err := s.db.QueryRowContext(ctx, `SELECT country, region, city, lat, lng, source, fetched_at FROM geo_cache WHERE ip_hash = ?`, ipHash).
		Scan(&info.Country, &info.Region, &info.City, &lat, &lng, &info.Source, &fetched)
	if err == sql.ErrNoRows {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t, _ := time.Parse(time.RFC3339Nano, fetched)
	if maxAge > 0 && time.Since(t) > maxAge {
		return nil, storage.ErrNotFound
	}
	if lat.Valid {
		info.Lat = lat.Float64
	}
	if lng.Valid {
		info.Lng = lng.Float64
	}
	return &info, nil
}

func (s *Store) PutGeoCache(ctx context.Context, ipHash string, info storage.GeoInfo) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO geo_cache(ip_hash, country, region, city, lat, lng, source, fetched_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(ip_hash) DO UPDATE SET country=excluded.country, region=excluded.region, city=excluded.city,
		lat=excluded.lat, lng=excluded.lng, source=excluded.source, fetched_at=excluded.fetched_at`,
		ipHash, info.Country, info.Region, info.City, info.Lat, info.Lng, info.Source, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) AddVisit(ctx context.Context, v storage.Visit) error {
	var lat, lng any
	if v.Lat != nil {
		lat = *v.Lat
	}
	if v.Lng != nil {
		lng = *v.Lng
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO visits(id, ip_hash, path, note_id, country, region, city, lat, lng, user_agent, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		v.ID, v.IPHash, v.Path, v.NoteID, v.Country, v.Region, v.City, lat, lng, v.UserAgent, v.CreatedAt)
	return err
}

func (s *Store) GetVisitStats(ctx context.Context, recentDays int) (*storage.VisitStats, error) {
	if recentDays <= 0 {
		recentDays = 14
	}
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	since := now.AddDate(0, 0, -recentDays+1).Format("2006-01-02")

	stats := &storage.VisitStats{}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM visits WHERE substr(created_at,1,10) = ?`, today).Scan(&stats.TodayCount); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM visits WHERE substr(created_at,1,10) >= ?`, since).Scan(&stats.RecentCount); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT substr(created_at,1,10) AS d, COUNT(1) FROM visits WHERE substr(created_at,1,10) >= ? GROUP BY d ORDER BY d ASC`, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d Daily
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.Daily = append(stats.Daily, storage.DailyCount{Date: d.Date, Count: d.Count})
	}
	rows.Close()

	locRows, err := s.db.QueryContext(ctx, `SELECT country, region, city, COALESCE(lat,0), COALESCE(lng,0), COUNT(1)
		FROM visits WHERE lat IS NOT NULL AND lng IS NOT NULL AND substr(created_at,1,10) >= ?
		GROUP BY country, region, city, lat, lng ORDER BY COUNT(1) DESC LIMIT 500`, since)
	if err != nil {
		return nil, err
	}
	for locRows.Next() {
		var loc storage.VisitLocation
		if err := locRows.Scan(&loc.Country, &loc.Region, &loc.City, &loc.Lat, &loc.Lng, &loc.Count); err != nil {
			locRows.Close()
			return nil, err
		}
		stats.Locations = append(stats.Locations, loc)
	}
	locRows.Close()

	noteRows, err := s.db.QueryContext(ctx, `SELECT id, title, like_count, comment_count, visible FROM notes ORDER BY like_count DESC, comment_count DESC`)
	if err != nil {
		return nil, err
	}
	defer noteRows.Close()
	for noteRows.Next() {
		var n storage.NoteEngagement
		var visible int
		if err := noteRows.Scan(&n.NoteID, &n.Title, &n.LikeCount, &n.CommentCount, &visible); err != nil {
			return nil, err
		}
		n.Visible = visible == 1
		stats.NoteStats = append(stats.NoteStats, n)
	}
	return stats, noteRows.Err()
}

type Daily struct {
	Date  string
	Count int64
}
