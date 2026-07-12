package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"timenotesblog/internal/auth"
	"timenotesblog/internal/geo"
	"timenotesblog/internal/protocol"
	"timenotesblog/internal/storage"
)

type Options struct {
	Addr                     string
	NotesDir                 string
	CoversDir                string
	SiteDir                  string
	JWTSecret                string
	PasswordPepper           string
	IPHashPepper             string
	MaxUploadBytes           int64
	MaxMessageBytes          int64
	PowBaseDifficulty        int
	PowMaxDifficulty         int
	JWTExpiry                time.Duration
	ReadDeadline             time.Duration
	MaxWSConnPerIPPerMinute  int
	MaxLoginPerIPPerMinute   int
	MaxCommentPerIPPerMinute int
	MaxChallengePerIPPerMinute int
	UploadTTL                time.Duration
	LimiterIdleTTL           time.Duration
	TrustedProxies           []string
	AdminPathToken           string
	GeoCacheTTL              time.Duration
	AllowOrigin              func(origin string) bool
	PublicBaseURL            string
}

type Hub struct {
	store        storage.Store
	opts         Options
	pow          *auth.PoWManager
	geo          geo.Provider
	events       *eventHub
	uploads      sync.Map // uploadID -> *uploadState
	wsLimit      sync.Map // ip -> *rateBucket
	loginLimit   sync.Map
	commentLimit sync.Map
	challengeLimit sync.Map
	trusted      []*net.IPNet
	archive      ArchiveLimits
	stopCleanup  chan struct{}
	cleanupOnce  sync.Once
}

type rateBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type uploadState struct {
	mu        sync.Mutex
	ID        string
	UserID    string
	NoteID    string // empty for create, set for update
	Filename  string
	Title     string
	Size      int64
	TmpPath   string
	File      *os.File
	Received  int64
	NextIndex int
	CreatedAt time.Time
	ExpiresAt time.Time
	IsAdmin   bool
}

type clientSession struct {
	conn      *websocket.Conn
	ip        string
	ipHash    string
	wsSession string
	user      *auth.Claims
	send      chan protocol.Envelope
	hub       *Hub
	closed    chan struct{}
	closeOnce sync.Once
	eventsOn  bool
}

func NewHub(store storage.Store, geoProvider geo.Provider, opts Options) *Hub {
	if opts.MaxMessageBytes <= 0 {
		opts.MaxMessageBytes = 2 * 1024 * 1024
	}
	if opts.ReadDeadline <= 0 {
		opts.ReadDeadline = 60 * time.Second
	}
	if opts.JWTExpiry <= 0 {
		opts.JWTExpiry = 24 * time.Hour
	}
	if opts.MaxWSConnPerIPPerMinute <= 0 {
		opts.MaxWSConnPerIPPerMinute = 60
	}
	if opts.MaxChallengePerIPPerMinute <= 0 {
		opts.MaxChallengePerIPPerMinute = 30
	}
	if opts.GeoCacheTTL <= 0 {
		opts.GeoCacheTTL = 7 * 24 * time.Hour
	}
	if opts.UploadTTL <= 0 {
		opts.UploadTTL = 30 * time.Minute
	}
	if opts.LimiterIdleTTL <= 0 {
		opts.LimiterIdleTTL = time.Hour
	}
	if strings.TrimSpace(opts.CoversDir) == "" {
		opts.CoversDir = filepath.Join(filepath.Dir(opts.NotesDir), "covers")
	}
	if strings.TrimSpace(opts.SiteDir) == "" {
		opts.SiteDir = filepath.Join(filepath.Dir(opts.NotesDir), "site")
	}
	_ = os.MkdirAll(opts.NotesDir, 0o755)
	_ = os.MkdirAll(opts.CoversDir, 0o755)
	_ = os.MkdirAll(opts.SiteDir, 0o755)
	h := &Hub{
		store:       store,
		opts:        opts,
		pow:         auth.NewPoWManager(opts.PowBaseDifficulty, opts.PowMaxDifficulty),
		geo:         geoProvider,
		events:      newEventHub(),
		archive:     defaultArchiveLimits(opts.MaxUploadBytes),
		stopCleanup: make(chan struct{}),
	}
	for _, p := range opts.TrustedProxies {
		if _, n, err := net.ParseCIDR(p); err == nil {
			h.trusted = append(h.trusted, n)
		} else if ip := net.ParseIP(p); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			h.trusted = append(h.trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	go h.cleanupLoop()
	return h
}

func (h *Hub) Close() {
	h.cleanupOnce.Do(func() {
		close(h.stopCleanup)
	})
}

func (h *Hub) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-h.stopCleanup:
			return
		case <-ticker.C:
			h.cleanupUploads()
			h.cleanupLimiters()
			_ = h.store.DeleteExpiredDownloadTokens(context.Background(), time.Now())
		}
	}
}

func (h *Hub) cleanupUploads() {
	now := time.Now()
	h.uploads.Range(func(key, value any) bool {
		st := value.(*uploadState)
		st.mu.Lock()
		expired := now.After(st.ExpiresAt)
		path := st.TmpPath
		file := st.File
		st.mu.Unlock()
		if !expired {
			return true
		}
		h.uploads.Delete(key)
		if file != nil {
			_ = file.Close()
		}
		if path != "" {
			_ = os.Remove(path)
		}
		return true
	})
}

func (h *Hub) cleanupLimiters() {
	cutoff := time.Now().Add(-h.opts.LimiterIdleTTL)
	for _, m := range []*sync.Map{&h.wsLimit, &h.loginLimit, &h.commentLimit, &h.challengeLimit} {
		m.Range(func(key, value any) bool {
			b := value.(*rateBucket)
			if b.lastSeen.Before(cutoff) {
				m.Delete(key)
			}
			return true
		})
	}
}

func (h *Hub) RegisterRoutes(app *fiber.App) {
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	app.Get("/files/:token", h.handleDownload)
	app.Get("/covers/:id", h.handleCover)
	app.Get("/site/background", h.handleSiteBackground)
	app.Post("/api/site/background/upload", h.handleSiteBackgroundHTTPUpload)

	app.Use("/ws", func(c fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		origin := c.Get("Origin")
		if origin != "" && h.opts.AllowOrigin != nil && !h.opts.AllowOrigin(origin) {
			return fiber.ErrForbidden
		}
		ip := h.clientIP(c)
		if !h.allowWS(ip) {
			return fiber.ErrTooManyRequests
		}
		c.Locals("clientIP", ip)
		return c.Next()
	})

	app.Get("/ws", websocket.New(func(conn *websocket.Conn) {
		h.serveWS(conn)
	}, websocket.Config{
		ReadBufferSize:  1024 * 64,
		WriteBufferSize: 1024 * 64,
	}))
}

func (h *Hub) clientIP(c fiber.Ctx) string {
	remote := c.IP()
	xff := c.Get("X-Forwarded-For")
	if xff == "" || len(h.trusted) == 0 {
		return remote
	}
	rip := net.ParseIP(remote)
	if !h.isTrustedIP(rip) {
		return remote
	}
	parts := strings.Split(xff, ",")
	// Walk right-to-left, skipping trusted proxies, and keep the nearest client IP.
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			continue
		}
		if h.isTrustedIP(ip) {
			continue
		}
		return ip.String()
	}
	return remote
}

func (h *Hub) isTrustedIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range h.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (h *Hub) allowWS(ip string) bool {
	return allowRate(&h.wsLimit, ip, h.opts.MaxWSConnPerIPPerMinute)
}

func (h *Hub) allowLogin(ip string) bool {
	return allowRate(&h.loginLimit, ip, h.opts.MaxLoginPerIPPerMinute)
}

func (h *Hub) allowComment(ip string) bool {
	return allowRate(&h.commentLimit, ip, h.opts.MaxCommentPerIPPerMinute)
}

func (h *Hub) allowChallenge(ip string) bool {
	return allowRate(&h.challengeLimit, ip, h.opts.MaxChallengePerIPPerMinute)
}

func allowRate(m *sync.Map, ip string, perMin int) bool {
	if perMin <= 0 {
		return true
	}
	now := time.Now()
	v, _ := m.LoadOrStore(ip, &rateBucket{
		limiter:  rate.NewLimiter(rate.Every(time.Minute/time.Duration(perMin)), perMin),
		lastSeen: now,
	})
	b := v.(*rateBucket)
	b.lastSeen = now
	return b.limiter.Allow()
}

func (h *Hub) serveWS(conn *websocket.Conn) {
	ip, _ := conn.Locals("clientIP").(string)
	if ip == "" {
		ip = conn.IP()
	}
	if ip == "" {
		ip = "0.0.0.0"
	}
	cs := &clientSession{
		conn:      conn,
		ip:        ip,
		ipHash:    auth.HashIP(ip, h.opts.IPHashPepper),
		wsSession: protocol.NewID(),
		send:      make(chan protocol.Envelope, 64),
		hub:       h,
		closed:    make(chan struct{}),
	}
	h.events.add(cs)
	go cs.writeLoop()
	cs.readLoop()
}

func (cs *clientSession) close() {
	cs.closeOnce.Do(func() {
		cs.hub.events.remove(cs)
		close(cs.closed)
		_ = cs.conn.Close()
	})
}

func (cs *clientSession) writeLoop() {
	defer cs.close()
	for {
		select {
		case <-cs.closed:
			return
		case env, ok := <-cs.send:
			if !ok {
				return
			}
			_ = cs.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := cs.conn.WriteJSON(env); err != nil {
				return
			}
		}
	}
}

func (cs *clientSession) reply(env protocol.Envelope) {
	select {
	case cs.send <- env:
	case <-cs.closed:
	case <-time.After(2 * time.Second):
	}
}

func (cs *clientSession) replyOK(msgType, id string, payload any) {
	env, err := protocol.NewEnvelope(msgType, id, payload)
	if err != nil {
		cs.reply(protocol.NewError(id, "encode_error", err.Error()))
		return
	}
	cs.reply(env)
}

func (cs *clientSession) replyErr(id, code, msg string) {
	cs.reply(protocol.NewError(id, code, msg))
}

func (cs *clientSession) readLoop() {
	defer cs.close()
	h := cs.hub
	for {
		_ = cs.conn.SetReadDeadline(time.Now().Add(h.opts.ReadDeadline))
		cs.conn.SetReadLimit(h.opts.MaxMessageBytes)
		var env protocol.Envelope
		if err := cs.conn.ReadJSON(&env); err != nil {
			return
		}
		if env.Version == 0 {
			env.Version = protocol.Version
		}
		cs.handle(env)
	}
}

func (cs *clientSession) handle(env protocol.Envelope) {
	ctx := context.Background()
	switch env.Type {
	case protocol.TypeAuthPowChallenge:
		cs.handlePowChallenge(ctx, env)
	case protocol.TypeAuthLogin:
		cs.handleLogin(ctx, env)
	case protocol.TypeAuthSession:
		cs.handleSession(env)
	case protocol.TypeAuthPing:
		cs.handlePing(env)
	case protocol.TypeNotesList:
		cs.handleNotesList(ctx, env)
	case protocol.TypeNotesGet:
		cs.handleNotesGet(ctx, env)
	case protocol.TypeNotesUploadStart:
		cs.handleUploadStart(ctx, env, false, false)
	case protocol.TypeNotesUploadChunk:
		cs.handleUploadChunk(env)
	case protocol.TypeNotesUploadFinish:
		cs.handleUploadFinish(ctx, env, false)
	case protocol.TypeNotesUpdateStart:
		cs.handleUploadStart(ctx, env, true, false)
	case protocol.TypeNotesUpdateChunk:
		cs.handleUploadChunk(env)
	case protocol.TypeNotesUpdateFinish:
		cs.handleUploadFinish(ctx, env, true)
	case protocol.TypeNotesLike:
		cs.handleLike(ctx, env)
	case protocol.TypeNotesCommentsList:
		cs.handleCommentsList(ctx, env)
	case protocol.TypeNotesCommentCreate:
		cs.handleCommentCreate(ctx, env)
	case protocol.TypeVisitTrack:
		cs.handleVisit(ctx, env)
	case protocol.TypeAdminNotesList:
		cs.handleAdminNotesList(ctx, env)
	case protocol.TypeAdminNoteSetVisible:
		cs.handleAdminSetVisible(ctx, env)
	case protocol.TypeAdminNoteDelete:
		cs.handleAdminDeleteNote(ctx, env)
	case protocol.TypeAdminNoteUploadStart:
		cs.handleUploadStart(ctx, env, false, true)
	case protocol.TypeAdminNoteUploadChunk:
		cs.handleUploadChunk(env)
	case protocol.TypeAdminNoteUploadFinish:
		cs.handleUploadFinish(ctx, env, false)
	case protocol.TypeAdminUsersList:
		cs.handleAdminUsersList(ctx, env)
	case protocol.TypeAdminUserCreate:
		cs.handleAdminUserCreate(ctx, env)
	case protocol.TypeAdminUserDelete:
		cs.handleAdminUserDelete(ctx, env)
	case protocol.TypeAdminUserUpdate:
		cs.handleAdminUserUpdate(ctx, env)
	case protocol.TypeAdminSelfUpdate:
		cs.handleAdminSelfUpdate(ctx, env)
	case protocol.TypeAdminStats:
		cs.handleAdminStats(ctx, env)
	case protocol.TypeAdminNoteSetPublicDownload:
		cs.handleAdminSetPublicDownload(ctx, env)
	case protocol.TypeAdminNoteDownload:
		cs.handleAdminNoteDownload(ctx, env)
	case protocol.TypeSiteSettingsGet:
		cs.handleSiteSettingsGet(ctx, env)
	case protocol.TypeAdminSiteGet:
		cs.handleAdminSiteGet(ctx, env)
	case protocol.TypeAdminSiteUpdate:
		cs.handleAdminSiteUpdate(ctx, env)
	case protocol.TypeAdminSiteBgUpload:
		cs.handleAdminSiteBgUpload(ctx, env)
	case protocol.TypeEventsSubscribe:
		cs.handleEventsSubscribe(env)
	case protocol.TypeEventsUnsubscribe:
		cs.handleEventsUnsubscribe(env)
	default:
		cs.replyErr(env.ID, "unknown_type", "unknown message type")
	}
}

func (cs *clientSession) requireUser() (*auth.Claims, error) {
	if cs.user == nil {
		return nil, errors.New("unauthorized")
	}
	return cs.user, nil
}

func (cs *clientSession) requireCurrentUser(ctx context.Context) (*storage.User, error) {
	if cs.user == nil {
		return nil, errors.New("unauthorized")
	}
	u, err := cs.hub.store.GetUserByID(ctx, cs.user.UserID)
	if err != nil {
		cs.user = nil
		return nil, errors.New("unauthorized")
	}
	// Keep session claims aligned with live role/username.
	cs.user.Username = u.Username
	cs.user.Role = u.Role
	return u, nil
}

func (cs *clientSession) requireAdmin() (*auth.Claims, error) {
	u, err := cs.requireUser()
	if err != nil {
		return nil, err
	}
	if u.Role != "admin" {
		return nil, errors.New("forbidden")
	}
	return u, nil
}

func (cs *clientSession) requireCurrentAdmin(ctx context.Context) (*storage.User, error) {
	u, err := cs.requireCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	if u.Role != "admin" {
		return nil, errors.New("forbidden")
	}
	return u, nil
}

func (cs *clientSession) handlePowChallenge(ctx context.Context, env protocol.Envelope) {
	if !cs.hub.allowChallenge(cs.ip) {
		cs.replyErr(env.ID, "rate_limited", "too many challenges")
		return
	}
	failures, _, _ := cs.hub.store.GetLoginFailures(ctx, cs.ipHash)
	ch, err := cs.hub.pow.IssueBound(cs.ipHash, cs.wsSession, failures)
	if err != nil {
		cs.replyErr(env.ID, "pow_error", "failed to issue challenge")
		return
	}
	cs.replyOK(protocol.TypeAuthPowChallenge, env.ID, ch)
}

type loginReq struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	ChallengeID string `json:"challengeId"`
	Nonce       string `json:"nonce"`
	Token       string `json:"token"`
}

func (cs *clientSession) handleLogin(ctx context.Context, env protocol.Envelope) {
	if !cs.hub.allowLogin(cs.ip) {
		cs.replyErr(env.ID, "rate_limited", "too many login attempts")
		return
	}
	req, err := protocol.DecodePayload[loginReq](env)
	if err != nil {
		cs.replyErr(env.ID, "bad_payload", "invalid payload")
		return
	}
// Token re-auth path (no PoW when JWT still valid).
		if strings.TrimSpace(req.Token) != "" {
			claims, err := auth.ParseJWT(cs.hub.opts.JWTSecret, req.Token)
			if err != nil {
				cs.replyErr(env.ID, "invalid_token", "token invalid or expired")
				return
			}
			user, err := cs.hub.store.GetUserByID(ctx, claims.UserID)
			if err != nil {
				cs.replyErr(env.ID, "invalid_token", "token invalid or expired")
				return
			}
			claims.Username = user.Username
			claims.Role = user.Role
			cs.user = claims
			cs.replyOK(protocol.TypeAuthLogin, env.ID, map[string]any{
				"token":                 req.Token,
				"userId":                user.ID,
				"username":              user.Username,
				"role":                  user.Role,
				"canUpload":             user.CanUpload,
				"mustChangeCredentials": user.MustChangeCredentials,
				"expiresAt":             claims.Exp,
			})
			return
		}

		ok, err := cs.hub.pow.VerifyBound(req.ChallengeID, cs.ipHash, cs.wsSession, req.Nonce)
		if err != nil || !ok {
			_, _ = cs.hub.store.BumpLoginFailure(ctx, cs.ipHash, time.Now())
			cs.replyErr(env.ID, "pow_failed", "proof of work failed")
			return
		}
		user, err := cs.hub.store.GetUserByUsername(ctx, strings.TrimSpace(req.Username))
		if err != nil {
			_, _ = cs.hub.store.BumpLoginFailure(ctx, cs.ipHash, time.Now())
			cs.replyErr(env.ID, "auth_failed", "invalid username or password")
			return
		}
		match, err := auth.VerifyPassword(user.PasswordHash, req.Password, cs.hub.opts.PasswordPepper)
		if err != nil || !match {
			_, _ = cs.hub.store.BumpLoginFailure(ctx, cs.ipHash, time.Now())
			cs.replyErr(env.ID, "auth_failed", "invalid username or password")
			return
		}
		_ = cs.hub.store.ResetLoginFailures(ctx, cs.ipHash)
		claims := auth.NewClaims(user.ID, user.Username, user.Role, cs.hub.opts.JWTExpiry)
		token, err := auth.IssueJWT(cs.hub.opts.JWTSecret, claims)
		if err != nil {
			cs.replyErr(env.ID, "token_error", "failed to issue token")
			return
		}
		cs.user = &claims
		cs.replyOK(protocol.TypeAuthLogin, env.ID, map[string]any{
			"token":                 token,
			"userId":                user.ID,
			"username":              user.Username,
			"role":                  user.Role,
			"canUpload":             user.CanUpload,
			"mustChangeCredentials": user.MustChangeCredentials,
			"expiresAt":             claims.Exp,
		})
	}

	func (cs *clientSession) handleSession(env protocol.Envelope) {
		req, err := protocol.DecodePayload[struct {
			Token string `json:"token"`
		}](env)
		if err != nil || req.Token == "" {
			cs.replyErr(env.ID, "bad_payload", "token required")
			return
		}
		claims, err := auth.ParseJWT(cs.hub.opts.JWTSecret, req.Token)
		if err != nil {
			cs.replyErr(env.ID, "invalid_token", "token invalid or expired")
			return
		}
		user, err := cs.hub.store.GetUserByID(context.Background(), claims.UserID)
		if err != nil {
			cs.replyErr(env.ID, "invalid_token", "token invalid or expired")
			return
		}
		claims.Username = user.Username
		claims.Role = user.Role
		cs.user = claims
		cs.replyOK(protocol.TypeAuthSession, env.ID, map[string]any{
			"userId":                user.ID,
			"username":              user.Username,
			"role":                  user.Role,
			"canUpload":             user.CanUpload,
			"mustChangeCredentials": user.MustChangeCredentials,
			"expiresAt":             claims.Exp,
		})
	}

func (cs *clientSession) handlePing(env protocol.Envelope) {
	if _, err := cs.requireUser(); err != nil {
		cs.replyErr(env.ID, "unauthorized", "login required")
		return
	}
	cs.replyOK(protocol.TypeAuthPing, env.ID, map[string]any{"ok": true, "serverTime": time.Now().UTC().Format(time.RFC3339Nano)})
}

func (cs *clientSession) handleNotesList(ctx context.Context, env protocol.Envelope) {
	notes, err := cs.hub.store.ListVisibleNotes(ctx)
	if err != nil {
		cs.replyErr(env.ID, "db_error", "failed to list notes")
		return
	}
	out := make([]storage.Note, 0, len(notes))
	for i := range notes {
		n := publicNoteView(&notes[i])
		out = append(out, n)
	}
	cs.replyOK(protocol.TypeNotesList, env.ID, map[string]any{"notes": out})
}

func (cs *clientSession) issueDownloadURL(ctx context.Context, noteID, purpose string) (string, error) {
	if strings.TrimSpace(purpose) == "" {
		purpose = "read"
	}
	token := protocol.NewToken(24)
	exp := time.Now().Add(10 * time.Minute)
	if err := cs.hub.store.CreateDownloadToken(ctx, token, noteID, purpose, exp); err != nil {
		return "", err
	}
	return "/files/" + token, nil
}

func (cs *clientSession) canDownloadNote(ctx context.Context, note *storage.Note) bool {
	if note == nil {
		return false
	}
	if u, err := cs.requireCurrentUser(ctx); err == nil {
		if u.Role == "admin" || u.ID == note.OwnerUserID {
			return true
		}
	}
	return note.Visible && note.PublicDownload
}

func noteCoverURL(note *storage.Note) string {
	if note == nil || strings.TrimSpace(note.CoverPath) == "" {
		return ""
	}
	return "/covers/" + note.ID
}

func (cs *clientSession) handleNotesGet(ctx context.Context, env protocol.Envelope) {
	req, err := protocol.DecodePayload[struct {
		ID string `json:"id"`
	}](env)
	if err != nil || req.ID == "" {
		cs.replyErr(env.ID, "bad_payload", "id required")
		return
	}
	note, err := cs.hub.store.GetNote(ctx, req.ID)
	if err != nil {
		cs.replyErr(env.ID, "not_found", "note not found")
		return
	}
	if !note.Visible {
		if cs.user == nil || cs.user.Role != "admin" {
			cs.replyErr(env.ID, "not_found", "note not found")
			return
		}
	}
	note.CoverURL = noteCoverURL(note)
	// Reading always uses a short-lived "read" token for in-app rendering.
	// Explicit downloads use export/export_auth tokens and are enforced in handleDownload.
	readURL, err := cs.issueDownloadURL(ctx, note.ID, "read")
	if err != nil {
		cs.replyErr(env.ID, "token_error", "failed to issue download")
		return
	}
	note.DownloadURL = readURL
	canDownload := cs.canDownloadNote(ctx, note)
	exportURL := ""
	if canDownload {
		purpose := "export"
		if u, err := cs.requireCurrentUser(ctx); err == nil && (u.Role == "admin" || u.ID == note.OwnerUserID) {
			purpose = "export_auth"
		}
		exportURL, err = cs.issueDownloadURL(ctx, note.ID, purpose)
		if err != nil {
			cs.replyErr(env.ID, "token_error", "failed to issue download")
			return
		}
	}
	liked, _ := cs.hub.store.HasLiked(ctx, note.ID, cs.ipHash)
	cs.replyOK(protocol.TypeNotesGet, env.ID, map[string]any{
		"note":        note,
		"liked":       liked,
		"canDownload": canDownload,
		"exportUrl":   exportURL,
	})
}

type uploadStartReq struct {
	Filename string `json:"filename"`
	Title    string `json:"title"`
	Size     int64  `json:"size"`
	NoteID   string `json:"noteId"`
}

func safeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if strings.Contains(name, "..") {
		return ""
	}
	return name
}

func (cs *clientSession) handleUploadStart(ctx context.Context, env protocol.Envelope, isUpdate, forceAdmin bool) {
	dbUser, err := cs.requireCurrentUser(ctx)
	if err != nil {
		cs.replyErr(env.ID, "unauthorized", "login required")
		return
	}
	if forceAdmin && dbUser.Role != "admin" {
		cs.replyErr(env.ID, "forbidden", "admin required")
		return
	}
	if !dbUser.CanUpload && dbUser.Role != "admin" {
		cs.replyErr(env.ID, "forbidden", "upload not allowed")
		return
	}
	if dbUser.MustChangeCredentials {
		cs.replyErr(env.ID, "credentials_required", "change default credentials first")
		return
	}
	req, err := protocol.DecodePayload[uploadStartReq](env)
	if err != nil {
		cs.replyErr(env.ID, "bad_payload", "invalid payload")
		return
	}
	filename := safeFilename(req.Filename)
	if filename == "" || !strings.HasSuffix(strings.ToLower(filename), ".tnote") {
		cs.replyErr(env.ID, "bad_filename", "filename must be a .tnote file")
		return
	}
	if req.Size <= 0 || req.Size > cs.hub.opts.MaxUploadBytes {
		cs.replyErr(env.ID, "bad_size", "invalid file size")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSuffix(filename, filepath.Ext(filename))
	}

	noteID := ""
	if isUpdate {
		noteID = strings.TrimSpace(req.NoteID)
		if noteID == "" {
			cs.replyErr(env.ID, "bad_payload", "noteId required for update")
			return
		}
		note, err := cs.hub.store.GetNote(ctx, noteID)
		if err != nil {
			cs.replyErr(env.ID, "not_found", "note not found")
			return
		}
		if note.OwnerUserID != dbUser.ID && dbUser.Role != "admin" {
			cs.replyErr(env.ID, "forbidden", "not note owner")
			return
		}
	} else {
		if existing, err := cs.hub.store.GetNoteByOwnerFilename(ctx, dbUser.ID, filename); err == nil && existing != nil {
			cs.replyErr(env.ID, "conflict", "filename already exists; use update")
			return
		} else if err != nil && !errors.Is(err, storage.ErrNotFound) {
			cs.replyErr(env.ID, "db_error", "failed to check filename")
			return
		}
	}

	uploadID := protocol.NewID()
	tmp := filepath.Join(cs.hub.opts.NotesDir, ".tmp-"+uploadID+".part")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		cs.replyErr(env.ID, "io_error", "failed to create temp file")
		return
	}
	now := time.Now()
	st := &uploadState{
		ID:        uploadID,
		UserID:    dbUser.ID,
		NoteID:    noteID,
		Filename:  filename,
		Title:     title,
		Size:      req.Size,
		TmpPath:   tmp,
		File:      f,
		NextIndex: 0,
		CreatedAt: now,
		ExpiresAt: now.Add(cs.hub.opts.UploadTTL),
		IsAdmin:   forceAdmin || dbUser.Role == "admin",
	}
	cs.hub.uploads.Store(uploadID, st)
	respType := protocol.TypeNotesUploadStart
	if isUpdate {
		respType = protocol.TypeNotesUpdateStart
	}
	if forceAdmin {
		respType = protocol.TypeAdminNoteUploadStart
	}
	cs.replyOK(respType, env.ID, map[string]any{"uploadId": uploadID, "chunkSize": 256 * 1024})
}

type uploadChunkReq struct {
	UploadID string `json:"uploadId"`
	Index    int    `json:"index"`
	Data     string `json:"data"` // base64
}

func (cs *clientSession) handleUploadChunk(env protocol.Envelope) {
	if _, err := cs.requireCurrentUser(context.Background()); err != nil {
		cs.replyErr(env.ID, "unauthorized", "login required")
		return
	}
	req, err := protocol.DecodePayload[uploadChunkReq](env)
	if err != nil || req.UploadID == "" {
		cs.replyErr(env.ID, "bad_payload", "invalid chunk")
		return
	}
	v, ok := cs.hub.uploads.Load(req.UploadID)
	if !ok {
		cs.replyErr(env.ID, "not_found", "upload not found")
		return
	}
	st := v.(*uploadState)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.UserID != cs.user.UserID && cs.user.Role != "admin" {
		cs.replyErr(env.ID, "forbidden", "not your upload")
		return
	}
	if time.Now().After(st.ExpiresAt) {
		cs.replyErr(env.ID, "upload_expired", "upload expired")
		return
	}
	if req.Index != st.NextIndex {
		cs.replyErr(env.ID, "bad_chunk_index", fmt.Sprintf("expected chunk index %d", st.NextIndex))
		return
	}
	raw, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		cs.replyErr(env.ID, "bad_payload", "invalid base64")
		return
	}
	if int64(len(raw)) > 512*1024 {
		cs.replyErr(env.ID, "chunk_too_large", "chunk too large")
		return
	}
	if st.Received+int64(len(raw)) > st.Size {
		cs.replyErr(env.ID, "size_exceeded", "upload exceeds declared size")
		return
	}
	if _, err := st.File.Write(raw); err != nil {
		cs.replyErr(env.ID, "io_error", "write failed")
		return
	}
	st.Received += int64(len(raw))
	st.NextIndex++
	cs.replyOK(env.Type, env.ID, map[string]any{"uploadId": st.ID, "received": st.Received, "nextIndex": st.NextIndex})
}

type uploadFinishReq struct {
	UploadID string `json:"uploadId"`
	SHA256   string `json:"sha256"`
}

func (cs *clientSession) handleUploadFinish(ctx context.Context, env protocol.Envelope, isUpdate bool) {
	user, err := cs.requireCurrentUser(ctx)
	if err != nil {
		cs.replyErr(env.ID, "unauthorized", "login required")
		return
	}
	req, err := protocol.DecodePayload[uploadFinishReq](env)
	if err != nil || req.UploadID == "" {
		cs.replyErr(env.ID, "bad_payload", "invalid finish")
		return
	}
	v, ok := cs.hub.uploads.Load(req.UploadID)
	if !ok {
		cs.replyErr(env.ID, "not_found", "upload not found")
		return
	}
	st := v.(*uploadState)
	st.mu.Lock()
	defer func() {
		cs.hub.uploads.Delete(req.UploadID)
		if st.File != nil {
			_ = st.File.Close()
		}
		st.mu.Unlock()
	}()
	if st.UserID != user.ID && user.Role != "admin" {
		_ = os.Remove(st.TmpPath)
		cs.replyErr(env.ID, "forbidden", "not your upload")
		return
	}
	if time.Now().After(st.ExpiresAt) {
		_ = os.Remove(st.TmpPath)
		cs.replyErr(env.ID, "upload_expired", "upload expired")
		return
	}
	if st.Received != st.Size {
		_ = os.Remove(st.TmpPath)
		cs.replyErr(env.ID, "size_mismatch", fmt.Sprintf("received %d expected %d", st.Received, st.Size))
		return
	}
	_ = st.File.Close()
	st.File = nil
	sum, err := fileSHA256(st.TmpPath)
	if err != nil {
		_ = os.Remove(st.TmpPath)
		cs.replyErr(env.ID, "io_error", "hash failed")
		return
	}
	if req.SHA256 != "" && !strings.EqualFold(req.SHA256, sum) {
		_ = os.Remove(st.TmpPath)
		cs.replyErr(env.ID, "hash_mismatch", "sha256 mismatch")
		return
	}
	validated, err := ValidateTNoteArchive(st.TmpPath, cs.hub.archive)
	if err != nil {
		_ = os.Remove(st.TmpPath)
		if strings.Contains(err.Error(), "thumbnail_required") {
			cs.replyErr(env.ID, "thumbnail_required", "open the notebook in TimeNotes to generate a cover, then upload again")
			return
		}
		cs.replyErr(env.ID, "invalid_archive", err.Error())
		return
	}
	if strings.TrimSpace(st.Title) == "" {
		st.Title = validated.Title
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	noteID := st.NoteID
	if noteID == "" {
		noteID = protocol.NewID()
	}
	finalName := noteID + ".tnote"
	finalPath := filepath.Join(cs.hub.opts.NotesDir, finalName)
	coverExt := validated.ThumbnailExt
	if coverExt == "" {
		coverExt = ".png"
	}
	coverPath := filepath.Join(cs.hub.opts.CoversDir, noteID+coverExt)
	tmpFinal := finalPath + ".new"
	tmpCover := coverPath + ".new"
	if err := copyFile(st.TmpPath, tmpFinal); err != nil {
		_ = os.Remove(st.TmpPath)
		cs.replyErr(env.ID, "io_error", "finalize failed")
		return
	}
	if err := os.WriteFile(tmpCover, validated.ThumbnailBytes, 0o644); err != nil {
		_ = os.Remove(st.TmpPath)
		_ = os.Remove(tmpFinal)
		cs.replyErr(env.ID, "io_error", "cover write failed")
		return
	}

	var note *storage.Note
	if isUpdate || st.NoteID != "" {
		existing, err := cs.hub.store.GetNote(ctx, st.NoteID)
		if err != nil {
			_ = os.Remove(st.TmpPath)
			_ = os.Remove(tmpFinal)
			_ = os.Remove(tmpCover)
			cs.replyErr(env.ID, "not_found", "note not found")
			return
		}
		oldPath := existing.StoragePath
		oldCover := existing.CoverPath
		existing.Title = st.Title
		existing.StoragePath = finalPath
		existing.CoverPath = coverPath
		existing.SizeBytes = st.Size
		existing.SHA256 = sum
		existing.UpdatedAt = now
		if err := cs.hub.store.UpdateNoteFile(ctx, *existing); err != nil {
			_ = os.Remove(st.TmpPath)
			_ = os.Remove(tmpFinal)
			_ = os.Remove(tmpCover)
			cs.replyErr(env.ID, "db_error", "update failed")
			return
		}
		_ = os.Rename(tmpFinal, finalPath)
		_ = os.Rename(tmpCover, coverPath)
		if oldPath != "" && oldPath != finalPath {
			_ = os.Remove(oldPath)
		}
		if oldCover != "" && oldCover != coverPath {
			_ = os.Remove(oldCover)
		}
		note = existing
	} else {
		n := storage.Note{
			ID:             noteID,
			OwnerUserID:    user.ID,
			Filename:       st.Filename,
			Title:          st.Title,
			StoragePath:    finalPath,
			CoverPath:      coverPath,
			SizeBytes:      st.Size,
			SHA256:         sum,
			Visible:        true,
			PublicDownload: false,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := cs.hub.store.CreateNote(ctx, n); err != nil {
			_ = os.Remove(st.TmpPath)
			_ = os.Remove(tmpFinal)
			_ = os.Remove(tmpCover)
			if errors.Is(err, storage.ErrConflict) {
				cs.replyErr(env.ID, "conflict", "filename already exists")
				return
			}
			cs.replyErr(env.ID, "db_error", "create failed")
			return
		}
		_ = os.Rename(tmpFinal, finalPath)
		_ = os.Rename(tmpCover, coverPath)
		note = &n
	}
	_ = os.Remove(st.TmpPath)
	full, _ := cs.hub.store.GetNote(ctx, note.ID)
	if full != nil {
		note = full
	}
	note.CoverURL = noteCoverURL(note)
	cs.hub.events.broadcast(protocol.TypeEventNoteChanged, map[string]any{"note": publicNoteView(note)}, audiencePublic)
	cs.hub.events.broadcast(protocol.TypeEventNoteChanged, map[string]any{"note": note}, audienceAdmin)
	respType := protocol.TypeNotesUploadFinish
	if isUpdate {
		respType = protocol.TypeNotesUpdateFinish
	}
	cs.replyOK(respType, env.ID, map[string]any{"note": note})
}

func publicNoteView(note *storage.Note) storage.Note {
	if note == nil {
		return storage.Note{}
	}
	out := *note
	out.StoragePath = ""
	out.CoverPath = ""
	out.DownloadURL = ""
	out.CoverURL = noteCoverURL(note)
	return out
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (cs *clientSession) handleLike(ctx context.Context, env protocol.Envelope) {
	req, err := protocol.DecodePayload[struct {
		ID string `json:"id"`
	}](env)
	if err != nil || req.ID == "" {
		cs.replyErr(env.ID, "bad_payload", "id required")
		return
	}
	note, err := cs.hub.store.GetNote(ctx, req.ID)
	if err != nil || !note.Visible {
		cs.replyErr(env.ID, "not_found", "note not found")
		return
	}
	if err := cs.hub.store.AddLike(ctx, req.ID, cs.ipHash); err != nil {
		if errors.Is(err, storage.ErrAlreadyLiked) {
			cs.replyErr(env.ID, "already_liked", "already liked")
			return
		}
		cs.replyErr(env.ID, "db_error", "like failed")
		return
	}
note, _ = cs.hub.store.GetNote(ctx, req.ID)
		cs.hub.events.broadcast(protocol.TypeEventLikeChanged, map[string]any{
			"noteId":    req.ID,
			"likeCount": note.LikeCount,
		}, audiencePublic)
		cs.replyOK(protocol.TypeNotesLike, env.ID, map[string]any{"likeCount": note.LikeCount, "liked": true})
	}

func (cs *clientSession) handleCommentsList(ctx context.Context, env protocol.Envelope) {
	req, err := protocol.DecodePayload[struct {
		ID string `json:"id"`
	}](env)
	if err != nil || req.ID == "" {
		cs.replyErr(env.ID, "bad_payload", "id required")
		return
	}
	comments, err := cs.hub.store.ListComments(ctx, req.ID)
	if err != nil {
		cs.replyErr(env.ID, "db_error", "list failed")
		return
	}
	cs.replyOK(protocol.TypeNotesCommentsList, env.ID, map[string]any{"comments": comments})
}

type commentCreateReq struct {
	ID        string `json:"id"`
	Nickname  string `json:"nickname"`
	Email     string `json:"email"`
	GitHubURL string `json:"githubUrl"`
	Content   string `json:"content"`
}

func (cs *clientSession) handleCommentCreate(ctx context.Context, env protocol.Envelope) {
	if !cs.hub.allowComment(cs.ip) {
		cs.replyErr(env.ID, "rate_limited", "too many comments")
		return
	}
	req, err := protocol.DecodePayload[commentCreateReq](env)
	if err != nil || req.ID == "" {
		cs.replyErr(env.ID, "bad_payload", "invalid payload")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || len(content) > 2000 {
		cs.replyErr(env.ID, "bad_content", "content required (max 2000)")
		return
	}
	nickname := strings.TrimSpace(req.Nickname)
	email := strings.TrimSpace(req.Email)
	gh := strings.TrimSpace(req.GitHubURL)
	if gh != "" {
		if !isGitHubURL(gh) {
			cs.replyErr(env.ID, "bad_github", "invalid github url")
			return
		}
		if nickname == "" {
			nickname = githubUsername(gh)
		}
	} else {
		if nickname == "" || email == "" {
			cs.replyErr(env.ID, "bad_identity", "nickname and email required, or github url")
			return
		}
		if len(email) > 200 || !strings.Contains(email, "@") {
			cs.replyErr(env.ID, "bad_email", "invalid email")
			return
		}
	}
	if len(nickname) > 64 {
		nickname = nickname[:64]
	}
	note, err := cs.hub.store.GetNote(ctx, req.ID)
	if err != nil || !note.Visible {
		cs.replyErr(env.ID, "not_found", "note not found")
		return
	}
	c := storage.Comment{
		ID:        protocol.NewID(),
		NoteID:    req.ID,
		Nickname:  nickname,
		Email:     email,
		GitHubURL: gh,
		Content:   content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
if err := cs.hub.store.AddComment(ctx, c); err != nil {
			cs.replyErr(env.ID, "db_error", "create failed")
			return
		}
		if note, err := cs.hub.store.GetNote(ctx, req.ID); err == nil {
			cs.hub.events.broadcast(protocol.TypeEventCommentCreated, map[string]any{
				"noteId":       req.ID,
				"comment":      c,
				"commentCount": note.CommentCount,
			}, audiencePublic)
		}
		cs.replyOK(protocol.TypeNotesCommentCreate, env.ID, map[string]any{"comment": c})
	}

func isGitHubURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) >= 1 && parts[0] != ""
}

func githubUsername(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "github"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return "github"
	}
	return parts[0]
}

func (cs *clientSession) handleVisit(ctx context.Context, env protocol.Envelope) {
	req, err := protocol.DecodePayload[struct {
		Path      string `json:"path"`
		NoteID    string `json:"noteId"`
		UserAgent string `json:"userAgent"`
	}](env)
	if err != nil {
		cs.replyErr(env.ID, "bad_payload", "invalid payload")
		return
	}
	v := storage.Visit{
		ID:        protocol.NewID(),
		IPHash:    cs.ipHash,
		Path:      strings.TrimSpace(req.Path),
		NoteID:    strings.TrimSpace(req.NoteID),
		UserAgent: strings.TrimSpace(req.UserAgent),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
if info, err := cs.hub.store.GetGeoCache(ctx, cs.ipHash, cs.hub.opts.GeoCacheTTL); err == nil && info != nil {
			v.Country, v.Region, v.City = info.Country, info.Region, info.City
			lat, lng := info.Lat, info.Lng
			v.Lat, v.Lng = &lat, &lng
		} else {
			go cs.hub.resolveGeoAsync(cs.ip, cs.ipHash)
		}
		if err := cs.hub.store.AddVisit(ctx, v); err != nil {
			cs.replyErr(env.ID, "db_error", "visit failed")
			return
		}
		cs.hub.events.broadcast(protocol.TypeEventStatsChanged, map[string]any{"reason": "visit"}, audienceAdmin)
		cs.replyOK(protocol.TypeVisitTrack, env.ID, map[string]any{"ok": true})
	}

	func (h *Hub) resolveGeoAsync(ip, ipHash string) {
		// Public GeoIP APIs reject loopback/private ranges; skip to avoid noisy logs.
		if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed != nil {
			if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified() {
				return
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := h.store.GetGeoCache(ctx, ipHash, h.opts.GeoCacheTTL); err == nil {
			return
		}
		info, err := h.geo.Lookup(ctx, ip)
		if err != nil {
			log.Printf("geo lookup failed ip_hash=%s err=%v", shortHash(ipHash), err)
			return
		}
		_ = h.store.PutGeoCache(ctx, ipHash, info)
		_ = h.store.BackfillVisitGeo(ctx, ipHash, info)
		h.events.broadcast(protocol.TypeEventStatsChanged, map[string]any{"reason": "geo"}, audienceAdmin)
	}

func shortHash(h string) string {
	if len(h) >= 8 {
		return h[:8]
	}
	return h
}

func (cs *clientSession) handleAdminNotesList(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		notes, err := cs.hub.store.ListAllNotes(ctx)
		if err != nil {
			cs.replyErr(env.ID, "db_error", "list failed")
			return
		}
		for i := range notes {
			notes[i].CoverURL = noteCoverURL(&notes[i])
		}
		cs.replyOK(protocol.TypeAdminNotesList, env.ID, map[string]any{"notes": notes})
	}

	func (cs *clientSession) handleAdminSetVisible(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		req, err := protocol.DecodePayload[struct {
			ID      string `json:"id"`
			Visible bool   `json:"visible"`
		}](env)
		if err != nil || req.ID == "" {
			cs.replyErr(env.ID, "bad_payload", "invalid payload")
			return
		}
		if err := cs.hub.store.SetNoteVisible(ctx, req.ID, req.Visible); err != nil {
			cs.replyErr(env.ID, "db_error", "update failed")
			return
		}
		if note, err := cs.hub.store.GetNote(ctx, req.ID); err == nil {
			if req.Visible {
				cs.hub.events.broadcast(protocol.TypeEventNoteChanged, map[string]any{"note": publicNoteView(note)}, audiencePublic)
			} else {
				cs.hub.events.broadcast(protocol.TypeEventNoteDeleted, map[string]any{"id": req.ID}, audiencePublic)
			}
			cs.hub.events.broadcast(protocol.TypeEventNoteChanged, map[string]any{"note": note}, audienceAdmin)
		}
		cs.replyOK(protocol.TypeAdminNoteSetVisible, env.ID, map[string]any{"ok": true})
	}

	func (cs *clientSession) handleAdminSetPublicDownload(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		req, err := protocol.DecodePayload[struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}](env)
		if err != nil || req.ID == "" {
			cs.replyErr(env.ID, "bad_payload", "invalid payload")
			return
		}
		if err := cs.hub.store.SetNotePublicDownload(ctx, req.ID, req.Enabled); err != nil {
			cs.replyErr(env.ID, "db_error", "update failed")
			return
		}
		if note, err := cs.hub.store.GetNote(ctx, req.ID); err == nil {
			cs.hub.events.broadcast(protocol.TypeEventNoteChanged, map[string]any{"note": publicNoteView(note)}, audiencePublic)
			cs.hub.events.broadcast(protocol.TypeEventNoteChanged, map[string]any{"note": note}, audienceAdmin)
		}
		cs.replyOK(protocol.TypeAdminNoteSetPublicDownload, env.ID, map[string]any{"ok": true, "enabled": req.Enabled})
	}

	func (cs *clientSession) handleAdminNoteDownload(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		req, err := protocol.DecodePayload[struct {
			ID string `json:"id"`
		}](env)
		if err != nil || req.ID == "" {
			cs.replyErr(env.ID, "bad_payload", "id required")
			return
		}
		note, err := cs.hub.store.GetNote(ctx, req.ID)
		if err != nil {
			cs.replyErr(env.ID, "not_found", "note not found")
			return
		}
		urlPath, err := cs.issueDownloadURL(ctx, note.ID, "export_auth")
		if err != nil {
			cs.replyErr(env.ID, "token_error", "failed to issue download")
			return
		}
		cs.replyOK(protocol.TypeAdminNoteDownload, env.ID, map[string]any{
			"downloadUrl": urlPath,
			"filename":    note.Filename,
		})
	}

	func (cs *clientSession) handleAdminDeleteNote(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		req, err := protocol.DecodePayload[struct {
			ID string `json:"id"`
		}](env)
		if err != nil || req.ID == "" {
			cs.replyErr(env.ID, "bad_payload", "id required")
			return
		}
		note, err := cs.hub.store.GetNote(ctx, req.ID)
		if err != nil {
			cs.replyErr(env.ID, "not_found", "note not found")
			return
		}
		if err := cs.hub.store.DeleteNote(ctx, req.ID); err != nil {
			cs.replyErr(env.ID, "db_error", "delete failed")
			return
		}
		if note.StoragePath != "" {
			_ = os.Remove(note.StoragePath)
		}
		if note.CoverPath != "" {
			_ = os.Remove(note.CoverPath)
		}
		cs.hub.events.broadcast(protocol.TypeEventNoteDeleted, map[string]any{"id": req.ID}, audienceAll)
		cs.replyOK(protocol.TypeAdminNoteDelete, env.ID, map[string]any{"ok": true})
	}

	func (cs *clientSession) handleAdminUsersList(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		users, err := cs.hub.store.ListUsers(ctx)
		if err != nil {
			cs.replyErr(env.ID, "db_error", "list failed")
			return
		}
		cs.replyOK(protocol.TypeAdminUsersList, env.ID, map[string]any{"users": users})
	}

	func (cs *clientSession) handleAdminUserCreate(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
	req, err := protocol.DecodePayload[struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Role      string `json:"role"`
		CanUpload bool   `json:"canUpload"`
	}](env)
	if err != nil {
		cs.replyErr(env.ID, "bad_payload", "invalid payload")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || len(username) > 64 || strings.TrimSpace(req.Password) == "" {
		cs.replyErr(env.ID, "bad_payload", "username/password required")
		return
	}
	role := req.Role
	if role != "admin" {
		role = "user"
	}
	exists, err := cs.hub.store.UsernameExists(ctx, username, "")
	if err != nil {
		cs.replyErr(env.ID, "db_error", "check failed")
		return
	}
	if exists {
		cs.replyErr(env.ID, "conflict", "username exists")
		return
	}
	hash, err := auth.HashPassword(req.Password, cs.hub.opts.PasswordPepper)
	if err != nil {
		cs.replyErr(env.ID, "hash_error", "hash failed")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	u := storage.User{
		ID:           protocol.NewID(),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		CanUpload:    req.CanUpload || role == "admin",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := cs.hub.store.CreateUser(ctx, u); err != nil {
		cs.replyErr(env.ID, "db_error", "create failed")
		return
	}
u.PasswordHash = ""
		cs.hub.events.broadcast(protocol.TypeEventUserChanged, map[string]any{"user": u, "action": "create"}, audienceAdmin)
		cs.replyOK(protocol.TypeAdminUserCreate, env.ID, map[string]any{"user": u})
	}

	func (cs *clientSession) handleAdminUserDelete(ctx context.Context, env protocol.Envelope) {
		admin, err := cs.requireCurrentAdmin(ctx)
		if err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		req, err := protocol.DecodePayload[struct {
			ID               string `json:"id"`
			TransferToAdminID string `json:"transferToAdminId"`
		}](env)
		if err != nil || req.ID == "" {
			cs.replyErr(env.ID, "bad_payload", "id required")
			return
		}
		if req.ID == admin.ID {
			cs.replyErr(env.ID, "forbidden", "cannot delete self")
			return
		}
		if err := cs.hub.store.DeleteUserAndTransferNotes(ctx, req.ID, strings.TrimSpace(req.TransferToAdminID)); err != nil {
			switch {
			case errors.Is(err, storage.ErrLastAdmin):
				cs.replyErr(env.ID, "last_admin", "cannot delete the last admin")
			case errors.Is(err, storage.ErrInvalidInput):
				cs.replyErr(env.ID, "transfer_target_required", "transfer target admin required when user owns notes")
			case errors.Is(err, storage.ErrNotFound):
				cs.replyErr(env.ID, "not_found", "user or transfer target not found")
			case errors.Is(err, storage.ErrForbidden):
				cs.replyErr(env.ID, "forbidden", "transfer target must be an admin")
			default:
				cs.replyErr(env.ID, "db_error", "delete failed")
			}
			return
		}
		cs.hub.events.broadcast(protocol.TypeEventUserChanged, map[string]any{"id": req.ID, "action": "delete"}, audienceAdmin)
		cs.hub.events.broadcast(protocol.TypeEventNoteChanged, map[string]any{"reason": "ownership_transfer"}, audienceAdmin)
		cs.replyOK(protocol.TypeAdminUserDelete, env.ID, map[string]any{"ok": true})
	}

	func (cs *clientSession) handleAdminUserUpdate(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		req, err := protocol.DecodePayload[struct {
			ID        string  `json:"id"`
			Username  *string `json:"username"`
			Password  *string `json:"password"`
			Role      *string `json:"role"`
			CanUpload *bool   `json:"canUpload"`
		}](env)
		if err != nil || req.ID == "" {
			cs.replyErr(env.ID, "bad_payload", "invalid payload")
			return
		}
		u, err := cs.hub.store.GetUserByID(ctx, req.ID)
		if err != nil {
			cs.replyErr(env.ID, "not_found", "user not found")
			return
		}
		if req.Username != nil {
			name := strings.TrimSpace(*req.Username)
			if name == "" {
				cs.replyErr(env.ID, "bad_payload", "username empty")
				return
			}
			exists, err := cs.hub.store.UsernameExists(ctx, name, u.ID)
			if err != nil {
				cs.replyErr(env.ID, "db_error", "check failed")
				return
			}
			if exists {
				cs.replyErr(env.ID, "conflict", "username exists")
				return
			}
			u.Username = name
		}
		if req.Password != nil && *req.Password != "" {
			hash, err := auth.HashPassword(*req.Password, cs.hub.opts.PasswordPepper)
			if err != nil {
				cs.replyErr(env.ID, "hash_error", "hash failed")
				return
			}
			u.PasswordHash = hash
		}
		if req.Role != nil {
			nextRole := "user"
			if *req.Role == "admin" {
				nextRole = "admin"
			}
			if u.Role == "admin" && nextRole != "admin" {
				count, err := cs.hub.store.CountAdmins(ctx)
				if err != nil {
					cs.replyErr(env.ID, "db_error", "check failed")
					return
				}
				if count <= 1 {
					cs.replyErr(env.ID, "last_admin", "cannot demote the last admin")
					return
				}
			}
			u.Role = nextRole
			if nextRole == "admin" {
				u.CanUpload = true
			}
		}
		if req.CanUpload != nil {
			u.CanUpload = *req.CanUpload || u.Role == "admin"
		}
		u.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := cs.hub.store.UpdateUser(ctx, *u); err != nil {
			cs.replyErr(env.ID, "db_error", "update failed")
			return
		}
		u.PasswordHash = ""
		cs.hub.events.broadcast(protocol.TypeEventUserChanged, map[string]any{"user": u, "action": "update"}, audienceAdmin)
		cs.replyOK(protocol.TypeAdminUserUpdate, env.ID, map[string]any{"user": u})
	}

	func (cs *clientSession) handleAdminSelfUpdate(ctx context.Context, env protocol.Envelope) {
		admin, err := cs.requireCurrentAdmin(ctx)
		if err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		req, err := protocol.DecodePayload[struct {
			Username *string `json:"username"`
			Password *string `json:"password"`
		}](env)
		if err != nil {
			cs.replyErr(env.ID, "bad_payload", "invalid payload")
			return
		}
		u, err := cs.hub.store.GetUserByID(ctx, admin.ID)
		if err != nil {
			cs.replyErr(env.ID, "not_found", "user not found")
			return
		}
		usernameProvided := req.Username != nil && strings.TrimSpace(*req.Username) != ""
		passwordProvided := req.Password != nil && strings.TrimSpace(*req.Password) != ""
		if u.MustChangeCredentials && (!usernameProvided || !passwordProvided) {
			cs.replyErr(env.ID, "credentials_required", "both username and password are required")
			return
		}
		if usernameProvided {
			name := strings.TrimSpace(*req.Username)
			if name == "" {
				cs.replyErr(env.ID, "bad_payload", "username empty")
				return
			}
			if strings.EqualFold(name, "admin") && u.MustChangeCredentials {
				cs.replyErr(env.ID, "bad_payload", "choose a non-default username")
				return
			}
			exists, err := cs.hub.store.UsernameExists(ctx, name, u.ID)
			if err != nil || exists {
				cs.replyErr(env.ID, "conflict", "username exists")
				return
			}
			u.Username = name
		}
		if passwordProvided {
			if *req.Password == "123456" && u.MustChangeCredentials {
				cs.replyErr(env.ID, "bad_payload", "choose a non-default password")
				return
			}
			hash, err := auth.HashPassword(*req.Password, cs.hub.opts.PasswordPepper)
			if err != nil {
				cs.replyErr(env.ID, "hash_error", "hash failed")
				return
			}
			u.PasswordHash = hash
		}
		if u.MustChangeCredentials && usernameProvided && passwordProvided {
			u.MustChangeCredentials = false
		}
		u.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := cs.hub.store.UpdateUser(ctx, *u); err != nil {
			cs.replyErr(env.ID, "db_error", "update failed")
			return
		}
		if cs.user != nil {
			cs.user.Username = u.Username
		}
		cs.hub.events.broadcast(protocol.TypeEventUserChanged, map[string]any{"user": map[string]any{
			"id": u.ID, "username": u.Username, "role": u.Role, "canUpload": u.CanUpload, "mustChangeCredentials": u.MustChangeCredentials,
		}, "action": "self_update"}, audienceAdmin)
		cs.replyOK(protocol.TypeAdminSelfUpdate, env.ID, map[string]any{
			"ok":                    true,
			"username":              u.Username,
			"mustChangeCredentials": u.MustChangeCredentials,
		})
	}

	func (cs *clientSession) handleAdminStats(ctx context.Context, env protocol.Envelope) {
		if _, err := cs.requireCurrentAdmin(ctx); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
		stats, err := cs.hub.store.GetVisitStats(ctx, 14)
		if err != nil {
			cs.replyErr(env.ID, "db_error", "stats failed")
			return
		}
		cs.replyOK(protocol.TypeAdminStats, env.ID, stats)
	}

func mediaTypeFromPathOrURL(pathOrURL string) string {
	lower := strings.ToLower(pathOrURL)
	switch {
	case strings.HasSuffix(lower, ".mp4"), strings.Contains(lower, ".mp4?"):
		return "video"
	case strings.HasSuffix(lower, ".webm"), strings.Contains(lower, ".webm?"):
		return "video"
	case strings.HasSuffix(lower, ".mov"), strings.Contains(lower, ".mov?"):
		return "video"
	default:
		return "image"
	}
}

func (h *Hub) publicSiteSettings(st *storage.SiteSettings) map[string]any {
	if st == nil {
		return map[string]any{}
	}
	navTitle := strings.TrimSpace(st.NavTitle)
	if navTitle == "" {
		navTitle = "TimeNotes Blog"
	}
	out := map[string]any{
		"heroTitle":      st.HeroTitle,
		"heroSubtitle":   st.HeroSubtitle,
		"navTitle":       navTitle,
		"backgroundMode": st.BackgroundMode,
		"backgroundUrl":  st.BackgroundURL,
		"focusX":         st.FocusX,
		"focusY":         st.FocusY,
		"overlayColor":   st.OverlayColor,
		"overlayOpacity": st.OverlayOpacity,
		"updatedAt":      st.UpdatedAt,
	}
	mediaType := "image"
	if st.BackgroundMode == "upload" && strings.TrimSpace(st.BackgroundPath) != "" {
		// cache-bust when settings change so browsers pick up the new media
		out["backgroundAssetUrl"] = "/site/background?v=" + url.QueryEscape(st.UpdatedAt)
		mediaType = mediaTypeFromPathOrURL(st.BackgroundPath)
	} else if st.BackgroundMode == "url" && strings.TrimSpace(st.BackgroundURL) != "" {
		mediaType = mediaTypeFromPathOrURL(st.BackgroundURL)
	}
	out["backgroundMediaType"] = mediaType
	return out
}

func (cs *clientSession) handleSiteSettingsGet(ctx context.Context, env protocol.Envelope) {
	st, err := cs.hub.store.GetSiteSettings(ctx)
	if err != nil {
		cs.replyErr(env.ID, "db_error", "site settings failed")
		return
	}
	cs.replyOK(protocol.TypeSiteSettingsGet, env.ID, map[string]any{
		"settings": cs.hub.publicSiteSettings(st),
	})
}

func (cs *clientSession) handleAdminSiteGet(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireCurrentAdmin(ctx); err != nil {
		cs.replyErr(env.ID, "forbidden", "admin required")
		return
	}
	st, err := cs.hub.store.GetSiteSettings(ctx)
	if err != nil {
		cs.replyErr(env.ID, "db_error", "site settings failed")
		return
	}
	cs.replyOK(protocol.TypeAdminSiteGet, env.ID, map[string]any{
		"settings": cs.hub.publicSiteSettings(st),
	})
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func sanitizeOverlayColor(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return "#0b0d12"
	}
	if len(c) == 7 && c[0] == '#' {
		for i := 1; i < 7; i++ {
			ch := c[i]
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
				return "#0b0d12"
			}
		}
		return strings.ToLower(c)
	}
	return "#0b0d12"
}

func (cs *clientSession) handleAdminSiteUpdate(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireCurrentAdmin(ctx); err != nil {
		cs.replyErr(env.ID, "forbidden", "admin required")
		return
	}
	payload, err := protocol.DecodePayload[struct {
		HeroTitle      *string  `json:"heroTitle"`
		HeroSubtitle   *string  `json:"heroSubtitle"`
		NavTitle       *string  `json:"navTitle"`
		BackgroundMode *string  `json:"backgroundMode"`
		BackgroundURL  *string  `json:"backgroundUrl"`
		FocusX         *float64 `json:"focusX"`
		FocusY         *float64 `json:"focusY"`
		OverlayColor   *string  `json:"overlayColor"`
		OverlayOpacity *float64 `json:"overlayOpacity"`
		ClearUpload    bool     `json:"clearUpload"`
	}](env)
	if err != nil {
		cs.replyErr(env.ID, "bad_request", "invalid payload")
		return
	}
	cur, err := cs.hub.store.GetSiteSettings(ctx)
	if err != nil {
		cs.replyErr(env.ID, "db_error", "site settings failed")
		return
	}
	if payload.HeroTitle != nil {
		title := strings.TrimSpace(*payload.HeroTitle)
		if title == "" {
			title = "TimeNotes Blog"
		}
		if len(title) > 120 {
			cs.replyErr(env.ID, "bad_request", "hero title too long")
			return
		}
		cur.HeroTitle = title
	}
	if payload.HeroSubtitle != nil {
		sub := strings.TrimSpace(*payload.HeroSubtitle)
		if len(sub) > 300 {
			cs.replyErr(env.ID, "bad_request", "hero subtitle too long")
			return
		}
		cur.HeroSubtitle = sub
	}
	if payload.NavTitle != nil {
		nav := strings.TrimSpace(*payload.NavTitle)
		if nav == "" {
			nav = "TimeNotes Blog"
		}
		if len(nav) > 80 {
			cs.replyErr(env.ID, "bad_request", "nav title too long")
			return
		}
		cur.NavTitle = nav
	}
	if payload.BackgroundMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*payload.BackgroundMode))
		switch mode {
		case "none", "url", "upload":
			cur.BackgroundMode = mode
		default:
			cs.replyErr(env.ID, "bad_request", "invalid background mode")
			return
		}
	}
	if payload.BackgroundURL != nil {
		raw := strings.TrimSpace(*payload.BackgroundURL)
		if raw != "" {
			u, err := url.Parse(raw)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				cs.replyErr(env.ID, "bad_request", "background url must be http(s)")
				return
			}
			if len(raw) > 2048 {
				cs.replyErr(env.ID, "bad_request", "background url too long")
				return
			}
		}
		cur.BackgroundURL = raw
	}
	if payload.FocusX != nil {
		cur.FocusX = clampFloat(*payload.FocusX, 0, 100)
	}
	if payload.FocusY != nil {
		cur.FocusY = clampFloat(*payload.FocusY, 0, 100)
	}
	if payload.OverlayColor != nil {
		cur.OverlayColor = sanitizeOverlayColor(*payload.OverlayColor)
	}
	if payload.OverlayOpacity != nil {
		cur.OverlayOpacity = clampFloat(*payload.OverlayOpacity, 0, 0.9)
	}
	if payload.ClearUpload {
		if cur.BackgroundPath != "" {
			_ = os.Remove(cur.BackgroundPath)
		}
		cur.BackgroundPath = ""
		if cur.BackgroundMode == "upload" {
			cur.BackgroundMode = "none"
		}
	}
	// If mode is upload but no file exists, fall back to none.
	if cur.BackgroundMode == "upload" && strings.TrimSpace(cur.BackgroundPath) == "" {
		cur.BackgroundMode = "none"
	}
	// If mode is url but url empty, fall back to none.
	if cur.BackgroundMode == "url" && strings.TrimSpace(cur.BackgroundURL) == "" {
		cur.BackgroundMode = "none"
	}
	cur.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := cs.hub.store.UpdateSiteSettings(ctx, *cur); err != nil {
		cs.replyErr(env.ID, "db_error", "update site settings failed")
		return
	}
	pub := cs.hub.publicSiteSettings(cur)
	cs.hub.events.broadcast(protocol.TypeEventSiteSettingsChanged, map[string]any{"settings": pub}, audienceAll)
	cs.replyOK(protocol.TypeAdminSiteUpdate, env.ID, map[string]any{"settings": pub})
}

func (cs *clientSession) handleAdminSiteBgUpload(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireCurrentAdmin(ctx); err != nil {
		cs.replyErr(env.ID, "forbidden", "admin required")
		return
	}
	payload, err := protocol.DecodePayload[struct {
		Data string `json:"data"`
		Name string `json:"name"`
	}](env)
	if err != nil || strings.TrimSpace(payload.Data) == "" {
		cs.replyErr(env.ID, "bad_request", "invalid payload")
		return
	}
	// Allow optional data-url prefix.
	raw := payload.Data
	if i := strings.Index(raw, ","); i >= 0 && strings.Contains(strings.ToLower(raw[:i]), "base64") {
		raw = raw[i+1:]
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		cs.replyErr(env.ID, "bad_request", "invalid base64")
		return
	}
	// WS base64 path is for small images only; videos should use HTTP upload.
	const maxHeroBytes = 5 * 1024 * 1024
	if len(data) == 0 || int64(len(data)) > maxHeroBytes {
		cs.replyErr(env.ID, "bad_request", "media too large for WS upload; use file upload API (images ≤5MB, videos use HTTP)")
		return
	}
	ext, mime, kind, err := validateHeroMedia(data)
	if err != nil {
		cs.replyErr(env.ID, "bad_request", err.Error())
		return
	}
	_ = mime
	if kind == "video" {
		cs.replyErr(env.ID, "bad_request", "video must be uploaded via HTTP /api/site/background/upload")
		return
	}
	pub, err := cs.hub.commitSiteBackground(ctx, data, ext)
	if err != nil {
		cs.replyErr(env.ID, "io_error", err.Error())
		return
	}
	cs.replyOK(protocol.TypeAdminSiteBgUpload, env.ID, map[string]any{
		"settings": pub,
		"name":     payload.Name,
	})
}

func validateHeroMedia(data []byte) (ext, mime, kind string, err error) {
	if len(data) < 12 {
		return "", "", "", errors.New("file too small")
	}
	head := strings.ToLower(string(data[:min(len(data), 256)]))
	if strings.Contains(head, "<svg") || strings.Contains(head, "<html") || strings.Contains(head, "<!doctype") {
		return "", "", "", errors.New("svg/html not allowed")
	}
	if e, m, e2 := validateThumbnailImage(data, 8192*8192); e2 == nil {
		return e, m, "image", nil
	}
	// MP4 / ISO BMFF: ....ftyp
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		return ".mp4", "video/mp4", "video", nil
	}
	// WebM / Matroska EBML header 0x1A45DFA3
	if data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		return ".webm", "video/webm", "video", nil
	}
	return "", "", "", errors.New("invalid media: use png/jpg/webp/gif or mp4/webm")
}

func (h *Hub) commitSiteBackground(ctx context.Context, data []byte, ext string) (map[string]any, error) {
	cur, err := h.store.GetSiteSettings(ctx)
	if err != nil {
		return nil, err
	}
	oldPath := cur.BackgroundPath
	dest := filepath.Join(h.opts.SiteDir, "hero"+ext)
	tmp := dest + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return nil, errors.New("write failed")
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return nil, errors.New("commit failed")
	}
	if oldPath != "" && oldPath != dest {
		_ = os.Remove(oldPath)
	}
	cur.BackgroundPath = dest
	cur.BackgroundMode = "upload"
	cur.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := h.store.UpdateSiteSettings(ctx, *cur); err != nil {
		return nil, errors.New("update site settings failed")
	}
	pub := h.publicSiteSettings(cur)
	h.events.broadcast(protocol.TypeEventSiteSettingsChanged, map[string]any{"settings": pub}, audienceAll)
	return pub, nil
}

func (h *Hub) handleSiteBackgroundHTTPUpload(c fiber.Ctx) error {
	authz := c.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	if token == "" {
		token = strings.TrimSpace(c.Get("X-Admin-Token"))
	}
	if token == "" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	claims, err := auth.ParseJWT(h.opts.JWTSecret, token)
	if err != nil || claims == nil || claims.Role != "admin" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	u, err := h.store.GetUserByID(c.Context(), claims.UserID)
	if err != nil || u == nil || u.Role != "admin" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	file, err := c.FormFile("file")
	if err != nil || file == nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "file required"})
	}
	const maxVideo = 80 * 1024 * 1024
	if file.Size <= 0 || file.Size > maxVideo {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "file must be 1B–80MB"})
	}
	f, err := file.Open()
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "open failed"})
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxVideo+1))
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "read failed"})
	}
	if int64(len(data)) > maxVideo {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "file too large"})
	}
	ext, mime, kind, err := validateHeroMedia(data)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	_ = mime
	_ = kind
	pub, err := h.commitSiteBackground(c.Context(), data, ext)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"settings": pub})
}

	func (cs *clientSession) handleEventsSubscribe(env protocol.Envelope) {
		cs.eventsOn = true
		cs.replyOK(protocol.TypeEventsSubscribe, env.ID, map[string]any{"ok": true})
	}

	func (cs *clientSession) handleEventsUnsubscribe(env protocol.Envelope) {
		cs.eventsOn = false
		cs.replyOK(protocol.TypeEventsUnsubscribe, env.ID, map[string]any{"ok": true})
	}

	func (h *Hub) handleDownload(c fiber.Ctx) error {
		token := c.Params("token")
		if token == "" {
			return fiber.ErrNotFound
		}
		noteID, purpose, err := h.store.ConsumeDownloadToken(c.Context(), token)
		if err != nil {
			return fiber.ErrNotFound
		}
		note, err := h.store.GetNote(c.Context(), noteID)
		if err != nil {
			return fiber.ErrNotFound
		}
		switch purpose {
		case "export":
			// Public export is only valid when the note is visible and public download is enabled.
			if !note.Visible || !note.PublicDownload {
				return fiber.ErrNotFound
			}
		case "export_auth":
			// Auth-scoped export tokens (admin/owner) may download even if public download is off.
		case "read":
			// Read tokens are for in-app rendering of visible notes (or hidden notes for admins who received the token).
			if !note.Visible {
				return fiber.ErrNotFound
			}
		default:
			return fiber.ErrNotFound
		}
		filename := safeFilename(note.Filename)
		if filename == "" {
			filename = note.ID + ".tnote"
		}
		disposition := "inline"
		if purpose == "export" || purpose == "export_auth" {
			disposition = "attachment"
		}
		c.Set("Content-Type", "application/octet-stream")
		c.Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, strings.ReplaceAll(filename, `"`, "")))
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("Cache-Control", "no-store")
		return c.SendFile(note.StoragePath)
	}

	func (h *Hub) handleCover(c fiber.Ctx) error {
		id := strings.TrimSpace(c.Params("id"))
		if id == "" || strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
			return fiber.ErrNotFound
		}
		note, err := h.store.GetNote(c.Context(), id)
		if err != nil {
			return fiber.ErrNotFound
		}
		if !note.Visible {
			// Covers for hidden notes are admin-only; public cover endpoint hides existence.
			return fiber.ErrNotFound
		}
		if strings.TrimSpace(note.CoverPath) == "" {
			return fiber.ErrNotFound
		}
		switch strings.ToLower(filepath.Ext(note.CoverPath)) {
		case ".jpg", ".jpeg":
			c.Set("Content-Type", "image/jpeg")
		case ".webp":
			c.Set("Content-Type", "image/webp")
		case ".gif":
			c.Set("Content-Type", "image/gif")
		default:
			c.Set("Content-Type", "image/png")
		}
		c.Set("Cache-Control", "public, max-age=3600")
		return c.SendFile(note.CoverPath)
	}

func (h *Hub) handleSiteBackground(c fiber.Ctx) error {
	st, err := h.store.GetSiteSettings(c.Context())
	if err != nil || st == nil || st.BackgroundMode != "upload" || strings.TrimSpace(st.BackgroundPath) == "" {
		return fiber.ErrNotFound
	}
	// Ensure path is under SiteDir.
	absSite, _ := filepath.Abs(h.opts.SiteDir)
	absPath, err := filepath.Abs(st.BackgroundPath)
	if err != nil || !strings.HasPrefix(absPath, absSite+string(os.PathSeparator)) {
		return fiber.ErrNotFound
	}
	switch strings.ToLower(filepath.Ext(st.BackgroundPath)) {
	case ".jpg", ".jpeg":
		c.Set("Content-Type", "image/jpeg")
	case ".webp":
		c.Set("Content-Type", "image/webp")
	case ".gif":
		c.Set("Content-Type", "image/gif")
	case ".mp4":
		c.Set("Content-Type", "video/mp4")
	case ".webm":
		c.Set("Content-Type", "video/webm")
	case ".mov":
		c.Set("Content-Type", "video/quicktime")
	default:
		c.Set("Content-Type", "image/png")
	}
	c.Set("Cache-Control", "public, max-age=3600")
	return c.SendFile(st.BackgroundPath)
}

// MountStatic serves SPA for public and admin paths.
// Important: do NOT redirect between /admin/{token} and /admin/{token}/.
// Fiber's trailing-slash normalization would otherwise create ERR_TOO_MANY_REDIRECTS.
func MountStatic(app *fiber.App, webDir, adminToken string) {
	absWeb, err := filepath.Abs(webDir)
	if err != nil {
		absWeb = webDir
	}
	index := filepath.Join(absWeb, "index.html")
	adminPrefix := "/admin/" + adminToken

	serveSPA := func(c fiber.Ctx, rel string) error {
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || rel == "." {
			return c.SendFile(index)
		}
		// Prevent path traversal while resolving assets under web/.
		candidate := filepath.Join(absWeb, filepath.Clean("/"+rel))
		if !strings.HasPrefix(candidate, absWeb+string(os.PathSeparator)) && candidate != absWeb {
			return c.SendFile(index)
		}
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return c.SendFile(candidate)
		}
		return c.SendFile(index)
	}

	adminHandler := func(c fiber.Ctx) error {
		path := c.Path()
		// Accept both /admin/{token} and /admin/{token}/... without redirects.
		if path != adminPrefix && !strings.HasPrefix(path, adminPrefix+"/") {
			return c.Status(http.StatusNotFound).SendString("invalid admin path")
		}
		rel := strings.TrimPrefix(path, adminPrefix)
		return serveSPA(c, rel)
	}

	app.Get(adminPrefix, adminHandler)
	app.Get(adminPrefix+"/", adminHandler)
	app.Get(adminPrefix+"/*", adminHandler)

	app.Get("/*", func(c fiber.Ctx) error {
		path := c.Path()
		if strings.HasPrefix(path, "/ws") || strings.HasPrefix(path, "/files/") || strings.HasPrefix(path, "/covers/") || strings.HasPrefix(path, "/site/") || strings.HasPrefix(path, "/api/") || path == "/healthz" {
			return fiber.ErrNotFound
		}
		if strings.HasPrefix(path, "/admin/") {
			return c.Status(http.StatusNotFound).SendString("invalid admin path")
		}
		return serveSPA(c, path)
	})
}
