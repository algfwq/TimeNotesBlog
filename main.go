package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"

	"timenotesblog/internal/auth"
	"timenotesblog/internal/geo"
	"timenotesblog/internal/protocol"
	"timenotesblog/internal/server"
	"timenotesblog/internal/storage/sqlite"
)

// Frontend production build (npm run build → web/). Embedded into the binary so
// a single executable can be deployed without a sibling web/ directory.
//
//go:embed all:web
var embeddedWeb embed.FS

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	logFile, err := configureLogging(cfg.LogPath, cfg.LogMaxBytes)
	if err != nil {
		log.Fatalf("configure logging: %v", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	if err := os.MkdirAll(cfg.NotesDir, 0o755); err != nil {
		log.Fatalf("notes dir: %v", err)
	}

	store, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	jwtSecret := strings.TrimSpace(cfg.JWTSecret)
	allowWeak := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_ALLOW_WEAK_JWT")) == "1"
	if auth.IsWeakJWTSecret(jwtSecret) {
		if allowWeak {
			if jwtSecret == "" {
				jwtSecret = auth.EnsureSecret("")
			}
			log.Printf("WARNING: weak jwtSecret allowed via TIMENOTES_BLOG_ALLOW_WEAK_JWT=1 (dev only)")
		} else {
			log.Fatalf("jwtSecret is missing or too weak (min 16 chars, not a placeholder). Set jwtSecret in config or TIMENOTES_BLOG_JWT_SECRET. For local dev only: TIMENOTES_BLOG_ALLOW_WEAK_JWT=1")
		}
	}

	adminHash, err := auth.HashPassword("123456", cfg.PasswordPepper)
	if err != nil {
		log.Fatalf("hash default admin password: %v", err)
	}
	created, err := store.EnsureAdmin(context.Background(), "admin", adminHash)
	if err != nil {
		log.Fatalf("ensure admin: %v", err)
	}

	adminToken := protocol.NewToken(24)
	if _, err := rand.Read(make([]byte, 1)); err != nil {
		adminToken = hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	adminTokenShort := adminToken
	if len(adminTokenShort) > 8 {
		adminTokenShort = adminTokenShort[:8] + "…"
	}

	geoProvider := geo.NewProvider(geo.Config{
		Provider:     cfg.Geo.Provider,
		URLTemplate:  cfg.Geo.URLTemplate,
		APIKey:       cfg.Geo.APIKey,
		Timeout:      time.Duration(cfg.Geo.TimeoutMs) * time.Millisecond,
		CountryField: cfg.Geo.CountryField,
		RegionField:  cfg.Geo.RegionField,
		CityField:    cfg.Geo.CityField,
		LatField:     cfg.Geo.LatField,
		LngField:     cfg.Geo.LngField,
	})

	allowOriginFn := func(origin string) bool {
		return allowOrigin(origin, cfg.CORSOrigins, cfg.AllowLoopbackOrigins)
	}

	// BodyLimit must cover hero video uploads (up to 80MB) and note uploads.
	bodyLimit := int(cfg.MaxUploadBytes)
	if bodyLimit < 80*1024*1024 {
		bodyLimit = 80 * 1024 * 1024
	}
	app := fiber.New(fiber.Config{
		AppName:      "TimeNotes Blog Server",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		BodyLimit:    bodyLimit,
		// Keep trailing slash distinct so /admin/{token}/ is not normalized into a redirect loop.
		StrictRouting: true,
	})
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: allowOriginFn,
		AllowMethods:     []string{fiber.MethodGet, fiber.MethodPost, fiber.MethodOptions, fiber.MethodHead},
		AllowHeaders:     []string{"Content-Type", "Upgrade", "Connection", "Authorization", "X-Admin-Token"},
	}))

	hub := server.NewHub(store, geoProvider, server.Options{
		Addr:                     cfg.Addr,
		NotesDir:                 cfg.NotesDir,
		JWTSecret:                jwtSecret,
		PasswordPepper:           cfg.PasswordPepper,
		IPHashPepper:             cfg.IPHashPepper,
		MaxUploadBytes:           cfg.MaxUploadBytes,
		MaxMessageBytes:          cfg.MaxMessageBytes,
		PowBaseDifficulty:        cfg.PowBaseDifficulty,
		PowMaxDifficulty:         cfg.PowMaxDifficulty,
		JWTExpiry:                time.Duration(cfg.JWTExpiryHours) * time.Hour,
		ReadDeadline:             cfg.ReadDeadline,
		MaxWSConnPerIPPerMinute:  cfg.MaxWSConnPerIPPerMinute,
		MaxLoginPerIPPerMinute:   cfg.MaxLoginPerIPPerMinute,
		MaxCommentPerIPPerMinute: cfg.MaxCommentPerIPPerMinute,
		MaxVisitPerIPPerMinute:   cfg.MaxVisitPerIPPerMinute,
		MaxReadPerIPPerMinute:    cfg.MaxReadPerIPPerMinute,
		VisitRetentionDays:       cfg.VisitRetentionDays,
		TrustedProxies:           cfg.TrustedProxies,
		AdminPathToken:           adminToken,
		GeoCacheTTL:              time.Duration(cfg.Geo.CacheTTLHours) * time.Hour,
		AllowOrigin:              allowOriginFn,
	})
	hub.RegisterRoutes(app)

	webFS, webSource, err := openWebFS()
	if err != nil {
		log.Fatalf("web static assets: %v", err)
	}
	log.Printf("Serving frontend from %s", webSource)
	server.MountStatic(app, webFS, adminToken)

	log.Printf("TimeNotes Blog server config=%s addr=%s db=%s notes=%s", cfg.ConfigPath, cfg.Addr, cfg.DBPath, cfg.NotesDir)
	// Full admin URL only on stdout for operators; avoid repeating full token in rotated log files if multi-writer includes file.
	fmt.Fprintf(os.Stdout, "Admin UI: http://%s/admin/%s/\n", displayHost(cfg.Addr), adminToken)
	log.Printf("Admin UI path token prefix=%s (full URL printed once on stdout)", adminTokenShort)
	if created {
		fmt.Fprintln(os.Stdout, "Default admin account created: username=admin password=123456 (change immediately)")
		log.Printf("Default admin account created (change credentials immediately; password not written to log file)")
	} else {
		log.Printf("Admin account already exists in database (default password only applies on first start)")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("blog shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	if err := app.Listen(cfg.Addr); err != nil {
		log.Fatal(err)
	}
	log.Println("blog server stopped")
}

// openWebFS returns the frontend asset filesystem.
// Prefer on-disk web/ when it looks like a real Vite build (has assets/), so
// local frontend rebuilds can be tested without recompiling Go. Otherwise use
// the assets embedded at compile time — this is the production path and does
// not depend on the process working directory.
func openWebFS() (fs.FS, string, error) {
	diskIndex := filepath.Join("web", "index.html")
	diskAssets := filepath.Join("web", "assets")
	if st, err := os.Stat(diskIndex); err == nil && st.Size() > 0 {
		if ast, err := os.Stat(diskAssets); err == nil && ast.IsDir() {
			// Reject the old placeholder page that only asked to build frontend.
			raw, readErr := os.ReadFile(diskIndex)
			if readErr == nil && !strings.Contains(string(raw), "请构建前端") {
				return os.DirFS("web"), "disk:web/", nil
			}
		}
	}

	sub, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return nil, "", fmt.Errorf("embed web/: %w (run: cd frontend && npm run build)", err)
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, "", fmt.Errorf("embedded web/index.html missing (run: cd frontend && npm run build): %w", err)
	}
	// Require a real Vite build inside the binary (not a bare placeholder).
	if entries, err := fs.ReadDir(sub, "assets"); err != nil || len(entries) == 0 {
		return nil, "", fmt.Errorf("embedded web/assets missing or empty; run: cd frontend && npm run build")
	}
	return sub, "embed:web/", nil
}

func configureLogging(logPath string, maxBytes int64) (*os.File, error) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if logPath == "" {
		logPath = filepath.Join("logs", "timenotes-blog.log")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	if info, err := os.Stat(logPath); err == nil && maxBytes > 0 && info.Size() > maxBytes {
		if err := os.Truncate(logPath, 0); err != nil {
			return nil, err
		}
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, file))
	log.Printf("TimeNotes Blog log file: %s", logPath)
	return file, nil
}

func allowOrigin(origin string, configured []string, allowLoopback bool) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	for _, item := range configured {
		if strings.EqualFold(strings.TrimSpace(item), origin) {
			return true
		}
	}
	if !allowLoopback {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch parsed.Scheme {
	case "http", "https", "wails":
	default:
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "wails.localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func displayHost(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}
