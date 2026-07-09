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
	Addr                    string
	NotesDir                string
	JWTSecret               string
	PasswordPepper          string
	IPHashPepper            string
	MaxUploadBytes          int64
	MaxMessageBytes         int64
	PowBaseDifficulty       int
	PowMaxDifficulty        int
	JWTExpiry               time.Duration
	ReadDeadline            time.Duration
	MaxWSConnPerIPPerMinute int
	MaxLoginPerIPPerMinute  int
	MaxCommentPerIPPerMinute int
	TrustedProxies          []string
	AdminPathToken          string
	GeoCacheTTL             time.Duration
	AllowOrigin             func(origin string) bool
	PublicBaseURL           string
}

type Hub struct {
	store   storage.Store
	opts    Options
	pow     *auth.PoWManager
	geo     geo.Provider
	uploads sync.Map // uploadID -> *uploadState
	wsLimit sync.Map // ip -> *rate.Limiter
	loginLimit sync.Map
	commentLimit sync.Map
	trusted  []*net.IPNet
	mu       sync.Mutex
}

type uploadState struct {
	ID        string
	UserID    string
	NoteID    string // empty for create, set for update
	Filename  string
	Title     string
	Size      int64
	TmpPath   string
	File      *os.File
	Received  int64
	CreatedAt time.Time
	IsAdmin   bool
}

type clientSession struct {
	conn     *websocket.Conn
	ip       string
	ipHash   string
	user     *auth.Claims
	send     chan protocol.Envelope
	hub      *Hub
	closed   chan struct{}
	closeOnce sync.Once
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
	if opts.GeoCacheTTL <= 0 {
		opts.GeoCacheTTL = 7 * 24 * time.Hour
	}
	_ = os.MkdirAll(opts.NotesDir, 0o755)
	h := &Hub{
		store: store,
		opts:  opts,
		pow:   auth.NewPoWManager(opts.PowBaseDifficulty, opts.PowMaxDifficulty),
		geo:   geoProvider,
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
	return h
}

func (h *Hub) RegisterRoutes(app *fiber.App) {
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	app.Get("/files/:token", h.handleDownload)

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
	trusted := false
	for _, n := range h.trusted {
		if rip != nil && n.Contains(rip) {
			trusted = true
			break
		}
	}
	if !trusted {
		return remote
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	return remote
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

func allowRate(m *sync.Map, ip string, perMin int) bool {
	if perMin <= 0 {
		return true
	}
	v, _ := m.LoadOrStore(ip, rate.NewLimiter(rate.Every(time.Minute/time.Duration(perMin)), perMin))
	return v.(*rate.Limiter).Allow()
}

func (h *Hub) serveWS(conn *websocket.Conn) {
	ip := conn.IP()
	if ip == "" {
		ip = "0.0.0.0"
	}
	cs := &clientSession{
		conn:   conn,
		ip:     ip,
		ipHash: auth.HashIP(ip, h.opts.IPHashPepper),
		send:   make(chan protocol.Envelope, 64),
		hub:    h,
		closed: make(chan struct{}),
	}
	go cs.writeLoop()
	cs.readLoop()
}

func (cs *clientSession) close() {
	cs.closeOnce.Do(func() {
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

func (cs *clientSession) handlePowChallenge(ctx context.Context, env protocol.Envelope) {
	failures, _, _ := cs.hub.store.GetLoginFailures(ctx, cs.ipHash)
	ch, err := cs.hub.pow.Issue(failures)
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
		cs.user = claims
		cs.replyOK(protocol.TypeAuthLogin, env.ID, map[string]any{
			"token":    req.Token,
			"userId":   claims.UserID,
			"username": claims.Username,
			"role":     claims.Role,
			"expiresAt": claims.Exp,
		})
		return
	}

	ok, err := cs.hub.pow.Verify(req.ChallengeID, req.Nonce)
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
		"token":     token,
		"userId":    user.ID,
		"username":  user.Username,
		"role":      user.Role,
		"canUpload": user.CanUpload,
		"expiresAt": claims.Exp,
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
	cs.user = claims
	cs.replyOK(protocol.TypeAuthSession, env.ID, map[string]any{
		"userId":   claims.UserID,
		"username": claims.Username,
		"role":     claims.Role,
		"expiresAt": claims.Exp,
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
	cs.replyOK(protocol.TypeNotesList, env.ID, map[string]any{"notes": notes})
}

func (cs *clientSession) issueDownloadURL(ctx context.Context, noteID string) (string, error) {
	token := protocol.NewToken(24)
	exp := time.Now().Add(2 * time.Hour)
	if err := cs.hub.store.CreateDownloadToken(ctx, token, noteID, exp); err != nil {
		return "", err
	}
	return "/files/" + token, nil
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
	urlPath, err := cs.issueDownloadURL(ctx, note.ID)
	if err != nil {
		cs.replyErr(env.ID, "token_error", "failed to issue download")
		return
	}
	note.DownloadURL = urlPath
	liked, _ := cs.hub.store.HasLiked(ctx, note.ID, cs.ipHash)
	cs.replyOK(protocol.TypeNotesGet, env.ID, map[string]any{"note": note, "liked": liked})
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
	user, err := cs.requireUser()
	if err != nil {
		cs.replyErr(env.ID, "unauthorized", "login required")
		return
	}
	if forceAdmin {
		if _, err := cs.requireAdmin(); err != nil {
			cs.replyErr(env.ID, "forbidden", "admin required")
			return
		}
	}
	dbUser, err := cs.hub.store.GetUserByID(ctx, user.UserID)
	if err != nil {
		cs.replyErr(env.ID, "unauthorized", "user not found")
		return
	}
	if !dbUser.CanUpload && dbUser.Role != "admin" {
		cs.replyErr(env.ID, "forbidden", "upload not allowed")
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
		if note.OwnerUserID != user.UserID && user.Role != "admin" {
			cs.replyErr(env.ID, "forbidden", "not note owner")
			return
		}
	} else {
		if existing, err := cs.hub.store.GetNoteByOwnerFilename(ctx, user.UserID, filename); err == nil && existing != nil {
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
	st := &uploadState{
		ID:        uploadID,
		UserID:    user.UserID,
		NoteID:    noteID,
		Filename:  filename,
		Title:     title,
		Size:      req.Size,
		TmpPath:   tmp,
		File:      f,
		CreatedAt: time.Now(),
		IsAdmin:   forceAdmin || user.Role == "admin",
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
	if _, err := cs.requireUser(); err != nil {
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
	if st.UserID != cs.user.UserID && cs.user.Role != "admin" {
		cs.replyErr(env.ID, "forbidden", "not your upload")
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
	cs.replyOK(env.Type, env.ID, map[string]any{"uploadId": st.ID, "received": st.Received})
}

type uploadFinishReq struct {
	UploadID string `json:"uploadId"`
	SHA256   string `json:"sha256"`
}

func (cs *clientSession) handleUploadFinish(ctx context.Context, env protocol.Envelope, isUpdate bool) {
	user, err := cs.requireUser()
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
	defer func() {
		cs.hub.uploads.Delete(req.UploadID)
		_ = st.File.Close()
	}()
	if st.UserID != user.UserID && user.Role != "admin" {
		_ = os.Remove(st.TmpPath)
		cs.replyErr(env.ID, "forbidden", "not your upload")
		return
	}
	if st.Received != st.Size {
		_ = os.Remove(st.TmpPath)
		cs.replyErr(env.ID, "size_mismatch", fmt.Sprintf("received %d expected %d", st.Received, st.Size))
		return
	}
	_ = st.File.Close()
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	finalName := protocol.NewID() + ".tnote"
	finalPath := filepath.Join(cs.hub.opts.NotesDir, finalName)
	if err := os.Rename(st.TmpPath, finalPath); err != nil {
		// cross-device fallback
		if err2 := copyFile(st.TmpPath, finalPath); err2 != nil {
			_ = os.Remove(st.TmpPath)
			cs.replyErr(env.ID, "io_error", "finalize failed")
			return
		}
		_ = os.Remove(st.TmpPath)
	}

	var note *storage.Note
	if isUpdate || st.NoteID != "" {
		existing, err := cs.hub.store.GetNote(ctx, st.NoteID)
		if err != nil {
			_ = os.Remove(finalPath)
			cs.replyErr(env.ID, "not_found", "note not found")
			return
		}
		oldPath := existing.StoragePath
		existing.Title = st.Title
		existing.StoragePath = finalPath
		existing.SizeBytes = st.Size
		existing.SHA256 = sum
		existing.UpdatedAt = now
		if err := cs.hub.store.UpdateNoteFile(ctx, *existing); err != nil {
			_ = os.Remove(finalPath)
			cs.replyErr(env.ID, "db_error", "update failed")
			return
		}
		if oldPath != "" && oldPath != finalPath {
			_ = os.Remove(oldPath)
		}
		note = existing
	} else {
		n := storage.Note{
			ID:          protocol.NewID(),
			OwnerUserID: user.UserID,
			Filename:    st.Filename,
			Title:       st.Title,
			StoragePath: finalPath,
			SizeBytes:   st.Size,
			SHA256:      sum,
			Visible:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := cs.hub.store.CreateNote(ctx, n); err != nil {
			_ = os.Remove(finalPath)
			if errors.Is(err, storage.ErrConflict) {
				cs.replyErr(env.ID, "conflict", "filename already exists")
				return
			}
			cs.replyErr(env.ID, "db_error", "create failed")
			return
		}
		note = &n
	}
	full, _ := cs.hub.store.GetNote(ctx, note.ID)
	if full != nil {
		note = full
	}
	respType := protocol.TypeNotesUploadFinish
	if isUpdate {
		respType = protocol.TypeNotesUpdateFinish
	}
	cs.replyOK(respType, env.ID, map[string]any{"note": note})
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
	cs.replyOK(protocol.TypeVisitTrack, env.ID, map[string]any{"ok": true})
}

func (h *Hub) resolveGeoAsync(ip, ipHash string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := h.store.GetGeoCache(ctx, ipHash, h.opts.GeoCacheTTL); err == nil {
		return
	}
	info, err := h.geo.Lookup(ctx, ip)
	if err != nil {
		log.Printf("geo lookup failed ip_hash=%s err=%v", ipHash[:8], err)
		return
	}
	_ = h.store.PutGeoCache(ctx, ipHash, info)
}

func (cs *clientSession) handleAdminNotesList(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireAdmin(); err != nil {
		cs.replyErr(env.ID, "forbidden", "admin required")
		return
	}
	notes, err := cs.hub.store.ListAllNotes(ctx)
	if err != nil {
		cs.replyErr(env.ID, "db_error", "list failed")
		return
	}
	cs.replyOK(protocol.TypeAdminNotesList, env.ID, map[string]any{"notes": notes})
}

func (cs *clientSession) handleAdminSetVisible(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireAdmin(); err != nil {
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
	cs.replyOK(protocol.TypeAdminNoteSetVisible, env.ID, map[string]any{"ok": true})
}

func (cs *clientSession) handleAdminDeleteNote(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireAdmin(); err != nil {
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
	cs.replyOK(protocol.TypeAdminNoteDelete, env.ID, map[string]any{"ok": true})
}

func (cs *clientSession) handleAdminUsersList(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireAdmin(); err != nil {
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
	if _, err := cs.requireAdmin(); err != nil {
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
	cs.replyOK(protocol.TypeAdminUserCreate, env.ID, map[string]any{"user": u})
}

func (cs *clientSession) handleAdminUserDelete(ctx context.Context, env protocol.Envelope) {
	admin, err := cs.requireAdmin()
	if err != nil {
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
	if req.ID == admin.UserID {
		cs.replyErr(env.ID, "forbidden", "cannot delete self")
		return
	}
	if err := cs.hub.store.DeleteUser(ctx, req.ID); err != nil {
		cs.replyErr(env.ID, "db_error", "delete failed")
		return
	}
	cs.replyOK(protocol.TypeAdminUserDelete, env.ID, map[string]any{"ok": true})
}

func (cs *clientSession) handleAdminUserUpdate(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireAdmin(); err != nil {
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
		if *req.Role == "admin" {
			u.Role = "admin"
		} else {
			u.Role = "user"
		}
	}
	if req.CanUpload != nil {
		u.CanUpload = *req.CanUpload
	}
	u.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := cs.hub.store.UpdateUser(ctx, *u); err != nil {
		cs.replyErr(env.ID, "db_error", "update failed")
		return
	}
	u.PasswordHash = ""
	cs.replyOK(protocol.TypeAdminUserUpdate, env.ID, map[string]any{"user": u})
}

func (cs *clientSession) handleAdminSelfUpdate(ctx context.Context, env protocol.Envelope) {
	admin, err := cs.requireAdmin()
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
	u, err := cs.hub.store.GetUserByID(ctx, admin.UserID)
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
		if err != nil || exists {
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
	u.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := cs.hub.store.UpdateUser(ctx, *u); err != nil {
		cs.replyErr(env.ID, "db_error", "update failed")
		return
	}
	cs.replyOK(protocol.TypeAdminSelfUpdate, env.ID, map[string]any{"ok": true, "username": u.Username})
}

func (cs *clientSession) handleAdminStats(ctx context.Context, env protocol.Envelope) {
	if _, err := cs.requireAdmin(); err != nil {
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

func (h *Hub) handleDownload(c fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return fiber.ErrNotFound
	}
	noteID, exp, err := h.store.GetDownloadToken(c.Context(), token)
	if err != nil || time.Now().After(exp) {
		return fiber.ErrNotFound
	}
	note, err := h.store.GetNote(c.Context(), noteID)
	if err != nil {
		return fiber.ErrNotFound
	}
	if !note.Visible {
		// still allow with valid token (issued after auth/get)
	}
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, note.Filename))
	return c.SendFile(note.StoragePath)
}

// MountStatic serves SPA for public and admin paths.
func MountStatic(app *fiber.App, webDir, adminToken string) {
	index := filepath.Join(webDir, "index.html")
	app.Get("/admin/"+adminToken, func(c fiber.Ctx) error {
		return c.Redirect().To("/admin/" + adminToken + "/")
	})
	app.Get("/admin/"+adminToken+"/*", func(c fiber.Ctx) error {
		rel := strings.TrimPrefix(c.Path(), "/admin/"+adminToken)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return c.SendFile(index)
		}
		candidate := filepath.Join(webDir, filepath.Clean(rel))
		if strings.HasPrefix(candidate, webDir) {
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return c.SendFile(candidate)
			}
		}
		return c.SendFile(index)
	})

	app.Get("/*", func(c fiber.Ctx) error {
		path := c.Path()
		if strings.HasPrefix(path, "/ws") || strings.HasPrefix(path, "/files/") || path == "/healthz" {
			return fiber.ErrNotFound
		}
		if strings.HasPrefix(path, "/admin/") {
			return c.Status(http.StatusNotFound).SendString("invalid admin path")
		}
		rel := strings.TrimPrefix(path, "/")
		if rel == "" {
			return c.SendFile(index)
		}
		candidate := filepath.Join(webDir, filepath.Clean(rel))
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return c.SendFile(candidate)
		}
		return c.SendFile(index)
	})
}
