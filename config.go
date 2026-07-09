package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultConfigPath = "config.json"

type GeoConfig struct {
	Provider       string `json:"provider"`
	URLTemplate    string `json:"urlTemplate"`
	APIKey         string `json:"apiKey"`
	TimeoutMs      int    `json:"timeoutMs"`
	CacheTTLHours  int    `json:"cacheTTLHours"`
	CountryField   string `json:"countryField"`
	RegionField    string `json:"regionField"`
	CityField      string `json:"cityField"`
	LatField       string `json:"latField"`
	LngField       string `json:"lngField"`
}

type AppConfig struct {
	Addr                   string        `json:"addr"`
	DBPath                 string        `json:"dbPath"`
	NotesDir               string        `json:"notesDir"`
	LogPath                string        `json:"logPath"`
	LogMaxBytes            int64         `json:"logMaxBytes"`
	JWTSecret              string        `json:"jwtSecret"`
	PasswordPepper         string        `json:"passwordPepper"`
	IPHashPepper           string        `json:"ipHashPepper"`
	MaxUploadBytes         int64         `json:"maxUploadBytes"`
	MaxMessageBytes        int64         `json:"maxMessageBytes"`
	CORSOrigins            []string      `json:"corsOrigins"`
	AllowLoopbackOrigins   bool          `json:"allowLoopbackOrigins"`
	TrustedProxies         []string      `json:"trustedProxies"`
	PowBaseDifficulty      int           `json:"powBaseDifficulty"`
	PowMaxDifficulty       int           `json:"powMaxDifficulty"`
	JWTExpiryHours         int           `json:"jwtExpiryHours"`
	MaxWSConnPerIPPerMinute int          `json:"maxWSConnPerIPPerMinute"`
	MaxLoginPerIPPerMinute int           `json:"maxLoginPerIPPerMinute"`
	MaxCommentPerIPPerMinute int         `json:"maxCommentPerIPPerMinute"`
	ReadDeadline           time.Duration `json:"readDeadline"`
	Geo                    GeoConfig     `json:"geo"`
	ConfigPath             string        `json:"-"`
}

func loadConfig() (AppConfig, error) {
	cfg := defaultConfig()
	configPath := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_CONFIG"))
	if configPath == "" {
		configPath = defaultConfigPath
	}
	cfg.ConfigPath = configPath

	if body, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(body, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", configPath, err)
		}
		cfg.ConfigPath = configPath
	} else if !errors.Is(err, os.ErrNotExist) || strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_CONFIG")) != "" {
		return cfg, fmt.Errorf("read config %s: %w", configPath, err)
	}

	applyEnvOverrides(&cfg)
	return cfg, validateConfig(cfg)
}

func defaultConfig() AppConfig {
	return AppConfig{
		Addr:                     "127.0.0.1:8090",
		DBPath:                   filepath.Join("data", "blog.db"),
		NotesDir:                 filepath.Join("data", "notes"),
		LogPath:                  filepath.Join("logs", "timenotes-blog.log"),
		LogMaxBytes:              5 * 1024 * 1024,
		JWTSecret:                "",
		PasswordPepper:           "",
		IPHashPepper:             "timenotes-blog-ip",
		MaxUploadBytes:           100 * 1024 * 1024,
		MaxMessageBytes:          2 * 1024 * 1024,
		CORSOrigins:              []string{},
		AllowLoopbackOrigins:     true,
		PowBaseDifficulty:        4,
		PowMaxDifficulty:         20,
		JWTExpiryHours:           24,
		MaxWSConnPerIPPerMinute:  60,
		MaxLoginPerIPPerMinute:   20,
		MaxCommentPerIPPerMinute: 30,
		ReadDeadline:             60 * time.Second,
		Geo: GeoConfig{
			Provider:      "ip-api",
			URLTemplate:   "http://ip-api.com/json/{ip}?fields=status,message,country,regionName,city,lat,lon,query",
			TimeoutMs:     3000,
			CacheTTLHours: 168,
			CountryField:  "country",
			RegionField:   "regionName",
			CityField:     "city",
			LatField:      "lat",
			LngField:      "lon",
		},
	}
}

func applyEnvOverrides(cfg *AppConfig) {
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_ADDR")); v != "" {
		cfg.Addr = v
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_DB")); v != "" {
		cfg.DBPath = v
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_NOTES_DIR")); v != "" {
		cfg.NotesDir = v
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_LOG")); v != "" {
		cfg.LogPath = v
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_JWT_SECRET")); v != "" {
		cfg.JWTSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_CORS_ORIGINS")); v != "" {
		cfg.CORSOrigins = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_LOG_MAX_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.LogMaxBytes = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_MAX_UPLOAD_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.MaxUploadBytes = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_GEO_URL")); v != "" {
		cfg.Geo.URLTemplate = v
		if cfg.Geo.Provider == "" || cfg.Geo.Provider == "ip-api" {
			cfg.Geo.Provider = "http_json"
		}
	}
	if v := strings.TrimSpace(os.Getenv("TIMENOTES_BLOG_GEO_API_KEY")); v != "" {
		cfg.Geo.APIKey = v
	}
}

func validateConfig(cfg AppConfig) error {
	if strings.TrimSpace(cfg.Addr) == "" {
		return errors.New("addr is required")
	}
	if cfg.MaxUploadBytes < 1024*1024 {
		return errors.New("maxUploadBytes must be >= 1MB")
	}
	if cfg.PowBaseDifficulty < 1 {
		return errors.New("powBaseDifficulty must be >= 1")
	}
	if cfg.PowMaxDifficulty < cfg.PowBaseDifficulty {
		return errors.New("powMaxDifficulty must be >= powBaseDifficulty")
	}
	if cfg.JWTExpiryHours < 1 {
		return errors.New("jwtExpiryHours must be >= 1")
	}
	if cfg.Geo.TimeoutMs <= 0 {
		cfg.Geo.TimeoutMs = 3000
	}
	if cfg.Geo.CacheTTLHours <= 0 {
		cfg.Geo.CacheTTLHours = 168
	}
	return nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
