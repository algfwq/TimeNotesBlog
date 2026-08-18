package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"timenotesblog/internal/storage"
)

type Config struct {
	Provider      string
	URLTemplate   string
	APIKey        string
	Timeout       time.Duration
	CountryField  string
	RegionField   string
	CityField     string
	LatField      string
	LngField      string
}

type Provider interface {
	Lookup(ctx context.Context, ip string) (storage.GeoInfo, error)
}

type HTTPProvider struct {
	cfg    Config
	client *http.Client
}

func NewProvider(cfg Config) Provider {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.CountryField == "" {
		cfg.CountryField = "country"
	}
	if cfg.RegionField == "" {
		cfg.RegionField = "region"
	}
	if cfg.CityField == "" {
		cfg.CityField = "city"
	}
	if cfg.LatField == "" {
		cfg.LatField = "latitude"
	}
	if cfg.LngField == "" {
		cfg.LngField = "longitude"
	}
	if cfg.URLTemplate == "" {
		// 默认 ipwho.is：免 API key、原生 HTTPS（访客 IP 不明文出网）、免费 1000 次/天/出口 IP，
		// 字段名与下方默认 field 配置直接对齐。ip-api 付费版可改回其 https 模板并填 {apiKey}。
		cfg.URLTemplate = "https://ipwho.is/{ip}"
		cfg.Provider = "ipwhois"
	}
	return &HTTPProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *HTTPProvider) Lookup(ctx context.Context, ip string) (storage.GeoInfo, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return storage.GeoInfo{}, fmt.Errorf("empty ip")
	}
	url := strings.ReplaceAll(p.cfg.URLTemplate, "{ip}", ip)
	url = strings.ReplaceAll(url, "{apiKey}", p.cfg.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return storage.GeoInfo{}, err
	}
	if p.cfg.APIKey != "" && !strings.Contains(p.cfg.URLTemplate, "{apiKey}") {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return storage.GeoInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return storage.GeoInfo{}, err
	}
	if resp.StatusCode >= 300 {
		return storage.GeoInfo{}, fmt.Errorf("geo status %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return storage.GeoInfo{}, err
	}
	if status, ok := raw["status"].(string); ok && status == "fail" {
		msg, _ := raw["message"].(string)
		return storage.GeoInfo{}, fmt.Errorf("geo fail: %s", msg)
	}
	// ipwho.is 用 success:false 表示查询失败（保留 IP、限流等），失败时不落缓存。
	if success, ok := raw["success"].(bool); ok && !success {
		msg, _ := raw["message"].(string)
		return storage.GeoInfo{}, fmt.Errorf("geo fail: %s", msg)
	}
	info := storage.GeoInfo{
		Country: asString(raw[p.cfg.CountryField]),
		Region:  asString(raw[p.cfg.RegionField]),
		City:    asString(raw[p.cfg.CityField]),
		Lat:     asFloat(raw[p.cfg.LatField]),
		Lng:     asFloat(raw[p.cfg.LngField]),
		Source:  p.cfg.Provider,
	}
	if info.Source == "" {
		info.Source = "http_json"
	}
	return info, nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	default:
		return ""
	}
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}
