package geo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestProvider(server *httptest.Server) Provider {
	return NewProvider(Config{
		URLTemplate:  server.URL + "/{ip}",
		Timeout:      2 * time.Second,
		CountryField: "country",
		RegionField:  "region",
		CityField:    "city",
		LatField:     "latitude",
		LngField:     "longitude",
	})
}

// TestLookupIpwhoisShape 验证默认 ipwho.is 响应格式能被默认字段配置正确解析。
func TestLookupIpwhoisShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"8.8.8.8","success":true,"country":"United States","region":"California","city":"San Jose","latitude":37.3361663,"longitude":-121.8905913}`))
	}))
	defer server.Close()

	info, err := newTestProvider(server).Lookup(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if info.Country != "United States" || info.Region != "California" || info.City != "San Jose" {
		t.Fatalf("unexpected location: %+v", info)
	}
	if info.Lat != 37.3361663 || info.Lng != -121.8905913 {
		t.Fatalf("unexpected coordinates: %+v", info)
	}
}

// TestLookupFailureFlag 验证 success:false（保留 IP/限流）被当作错误处理，不产生缓存污染。
func TestLookupFailureFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"192.168.1.1","success":false,"message":"Reserved range"}`))
	}))
	defer server.Close()

	if _, err := newTestProvider(server).Lookup(context.Background(), "192.168.1.1"); err == nil {
		t.Fatal("expected error for success:false response")
	}
}

// TestLookupLegacyIpApiFail 验证旧 ip-api 的 status:fail 标记仍被识别。
func TestLookupLegacyIpApiFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"fail","message":"invalid query"}`))
	}))
	defer server.Close()

	if _, err := newTestProvider(server).Lookup(context.Background(), "1.2.3.4"); err == nil {
		t.Fatal("expected error for status:fail response")
	}
}
