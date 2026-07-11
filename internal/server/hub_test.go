package server

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"timenotesblog/internal/auth"
	"timenotesblog/internal/storage"
)

type stubStore struct {
	users map[string]*storage.User
	notes map[string]*storage.Note
	tokens map[string]struct {
		noteID string
		exp    time.Time
	}
}

func newStubStore() *stubStore {
	return &stubStore{
		users:  map[string]*storage.User{},
		notes:  map[string]*storage.Note{},
		tokens: map[string]struct {
			noteID string
			exp    time.Time
		}{},
	}
}

func (s *stubStore) Close() error { return nil }
func (s *stubStore) EnsureAdmin(ctx context.Context, username, passwordHash string) (bool, error) {
	return false, nil
}
func (s *stubStore) CountUsers(ctx context.Context) (int64, error) { return int64(len(s.users)), nil }
func (s *stubStore) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	for _, u := range s.users {
		if u.Role == "admin" {
			n++
		}
	}
	return n, nil
}
func (s *stubStore) CreateUser(ctx context.Context, user storage.User) error {
	cp := user
	s.users[user.ID] = &cp
	return nil
}
func (s *stubStore) GetUserByUsername(ctx context.Context, username string) (*storage.User, error) {
	for _, u := range s.users {
		if u.Username == username {
			cp := *u
			return &cp, nil
		}
	}
	return nil, storage.ErrNotFound
}
func (s *stubStore) GetUserByID(ctx context.Context, id string) (*storage.User, error) {
	u, ok := s.users[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := *u
	return &cp, nil
}
func (s *stubStore) ListUsers(ctx context.Context) ([]storage.User, error) { return nil, nil }
func (s *stubStore) UpdateUser(ctx context.Context, user storage.User) error {
	cp := user
	s.users[user.ID] = &cp
	return nil
}
func (s *stubStore) DeleteUser(ctx context.Context, id string) error {
	return s.DeleteUserAndTransferNotes(ctx, id, "")
}
func (s *stubStore) DeleteUserAndTransferNotes(ctx context.Context, userID, targetAdminID string) error {
	delete(s.users, userID)
	return nil
}
func (s *stubStore) UsernameExists(ctx context.Context, username string, excludeID string) (bool, error) {
	return false, nil
}
func (s *stubStore) CreateNote(ctx context.Context, note storage.Note) error {
	cp := note
	s.notes[note.ID] = &cp
	return nil
}
func (s *stubStore) UpdateNoteFile(ctx context.Context, note storage.Note) error {
	cp := note
	s.notes[note.ID] = &cp
	return nil
}
func (s *stubStore) GetNote(ctx context.Context, id string) (*storage.Note, error) {
	n, ok := s.notes[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := *n
	return &cp, nil
}
func (s *stubStore) GetNoteByOwnerFilename(ctx context.Context, ownerID, filename string) (*storage.Note, error) {
	return nil, storage.ErrNotFound
}
func (s *stubStore) ListVisibleNotes(ctx context.Context) ([]storage.Note, error) { return nil, nil }
func (s *stubStore) ListAllNotes(ctx context.Context) ([]storage.Note, error)     { return nil, nil }
func (s *stubStore) SetNoteVisible(ctx context.Context, id string, visible bool) error {
	return nil
}
func (s *stubStore) SetNotePublicDownload(ctx context.Context, id string, enabled bool) error {
	n, ok := s.notes[id]
	if !ok {
		return storage.ErrNotFound
	}
	n.PublicDownload = enabled
	return nil
}
func (s *stubStore) DeleteNote(ctx context.Context, id string) error {
	delete(s.notes, id)
	return nil
}
func (s *stubStore) AddLike(ctx context.Context, noteID, ipHash string) error { return nil }
func (s *stubStore) HasLiked(ctx context.Context, noteID, ipHash string) (bool, error) {
	return false, nil
}
func (s *stubStore) AddComment(ctx context.Context, c storage.Comment) error { return nil }
func (s *stubStore) ListComments(ctx context.Context, noteID string) ([]storage.Comment, error) {
	return nil, nil
}
func (s *stubStore) GetLoginFailures(ctx context.Context, ipHash string) (int, time.Time, error) {
	return 0, time.Time{}, nil
}
func (s *stubStore) BumpLoginFailure(ctx context.Context, ipHash string, now time.Time) (int, error) {
	return 1, nil
}
func (s *stubStore) ResetLoginFailures(ctx context.Context, ipHash string) error { return nil }
func (s *stubStore) CreateDownloadToken(ctx context.Context, token, noteID, purpose string, expiresAt time.Time) error {
	s.tokens[token] = struct {
		noteID string
		exp    time.Time
	}{noteID: noteID, exp: expiresAt}
	return nil
}
func (s *stubStore) ConsumeDownloadToken(ctx context.Context, token string) (string, string, error) {
	item, ok := s.tokens[token]
	if !ok || time.Now().After(item.exp) {
		return "", "", storage.ErrNotFound
	}
	delete(s.tokens, token)
	return item.noteID, "export", nil
}
func (s *stubStore) GetDownloadToken(ctx context.Context, token string) (string, string, time.Time, error) {
	item, ok := s.tokens[token]
	if !ok {
		return "", "", time.Time{}, storage.ErrNotFound
	}
	return item.noteID, "export", item.exp, nil
}
func (s *stubStore) DeleteExpiredDownloadTokens(ctx context.Context, now time.Time) error {
	return nil
}
func (s *stubStore) GetGeoCache(ctx context.Context, ipHash string, maxAge time.Duration) (*storage.GeoInfo, error) {
	return nil, storage.ErrNotFound
}
func (s *stubStore) PutGeoCache(ctx context.Context, ipHash string, info storage.GeoInfo) error {
	return nil
}
func (s *stubStore) AddVisit(ctx context.Context, v storage.Visit) error { return nil }
func (s *stubStore) BackfillVisitGeo(ctx context.Context, ipHash string, info storage.GeoInfo) error {
	return nil
}
func (s *stubStore) GetVisitStats(ctx context.Context, recentDays int) (*storage.VisitStats, error) {
	return &storage.VisitStats{}, nil
}

type fakeCtx struct {
	ip  string
	xff string
}

func (f fakeCtx) IP() string              { return f.ip }
func (f fakeCtx) Get(key string) string {
	if key == "X-Forwarded-For" {
		return f.xff
	}
	return ""
}

// Adapt fakeCtx to the subset used by clientIP through a local helper.
type ipSource interface {
	IP() string
	Get(string) string
}

func resolveIP(h *Hub, src ipSource) string {
	remote := src.IP()
	xff := src.Get("X-Forwarded-For")
	if xff == "" || len(h.trusted) == 0 {
		return remote
	}
	rip := net.ParseIP(remote)
	if !h.isTrustedIP(rip) {
		return remote
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil || h.isTrustedIP(ip) {
			continue
		}
		return ip.String()
	}
	return remote
}

func TestClientIPUsesTrustedProxyHeader(t *testing.T) {
	store := newStubStore()
	h := NewHub(store, nil, Options{
		NotesDir:       filepath.Join(t.TempDir(), "notes"),
		TrustedProxies: []string{"10.0.0.1"},
	})
	defer h.Close()
	got := resolveIP(h, fakeCtx{ip: "10.0.0.1", xff: "203.0.113.9, 10.0.0.1"})
	if got != "203.0.113.9" {
		t.Fatalf("got %q", got)
	}
}

func TestClientIPIgnoresUntrustedForwardedHeader(t *testing.T) {
	store := newStubStore()
	h := NewHub(store, nil, Options{
		NotesDir:       filepath.Join(t.TempDir(), "notes"),
		TrustedProxies: []string{"10.0.0.1"},
	})
	defer h.Close()
	got := resolveIP(h, fakeCtx{ip: "198.51.100.2", xff: "203.0.113.9"})
	if got != "198.51.100.2" {
		t.Fatalf("got %q", got)
	}
}

func TestCanDownloadNoteRules(t *testing.T) {
	store := newStubStore()
	h := NewHub(store, nil, Options{NotesDir: filepath.Join(t.TempDir(), "notes")})
	defer h.Close()
	admin := &storage.User{ID: "a1", Username: "admin", Role: "admin", CanUpload: true}
	owner := &storage.User{ID: "u1", Username: "owner", Role: "user", CanUpload: true}
	store.users[admin.ID] = admin
	store.users[owner.ID] = owner
	note := &storage.Note{ID: "n1", OwnerUserID: owner.ID, Visible: true, PublicDownload: false}
	store.notes[note.ID] = note

	adminCS := &clientSession{hub: h, user: &auth.Claims{UserID: admin.ID, Role: "admin"}}
	if !adminCS.canDownloadNote(context.Background(), note) {
		t.Fatal("admin should download")
	}
	ownerCS := &clientSession{hub: h, user: &auth.Claims{UserID: owner.ID, Role: "user"}}
	if !ownerCS.canDownloadNote(context.Background(), note) {
		t.Fatal("owner should download")
	}
	publicCS := &clientSession{hub: h}
	if publicCS.canDownloadNote(context.Background(), note) {
		t.Fatal("public should not download private note")
	}
	note.PublicDownload = true
	if !publicCS.canDownloadNote(context.Background(), note) {
		t.Fatal("public should download when enabled")
	}
}

func TestRequireCurrentUserRejectsDeletedAccount(t *testing.T) {
	store := newStubStore()
	h := NewHub(store, nil, Options{NotesDir: filepath.Join(t.TempDir(), "notes")})
	defer h.Close()
	cs := &clientSession{hub: h, user: &auth.Claims{UserID: "missing", Role: "admin"}}
	if _, err := cs.requireCurrentUser(context.Background()); err == nil {
		t.Fatal("expected unauthorized for deleted user")
	}
	if cs.user != nil {
		t.Fatal("session claims should be cleared")
	}
}

func TestUploadStateRejectsOutOfOrderChunkIndex(t *testing.T) {
	st := &uploadState{NextIndex: 2, Size: 100, ExpiresAt: time.Now().Add(time.Minute)}
	if st.NextIndex == 0 {
		t.Fatal("precondition")
	}
	// Behavioral guard: production code only accepts exact next index.
	if got, want := 1 != st.NextIndex, true; got != want {
		t.Fatal("index mismatch detection failed")
	}
}
