package ipinfo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/ipinfo-cli/ipinfo"
)

func newTestClient(ts *httptest.Server) *ipinfo.Client {
	cfg := ipinfo.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 1
	return ipinfo.NewClient(cfg)
}

// TestUserAgent checks that every request carries ipinfo-cli in User-Agent.
func TestUserAgent(t *testing.T) {
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		resp := map[string]any{"ip": "8.8.8.8"}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, _ = c.LookupIP(context.Background(), "8.8.8.8")

	if !strings.Contains(gotUA, "ipinfo-cli") {
		t.Errorf("User-Agent = %q, want it to contain ipinfo-cli", gotUA)
	}
}

// TestLookupIP checks that a single IP lookup is parsed correctly, including
// lat/lon splitting from the "loc" field.
func TestLookupIP(t *testing.T) {
	fixture := map[string]any{
		"ip":       "8.8.8.8",
		"city":     "Mountain View",
		"region":   "California",
		"country":  "US",
		"loc":      "37.3860,-122.0838",
		"org":      "AS15169 Google LLC",
		"postal":   "94035",
		"timezone": "America/Los_Angeles",
		"hostname": "dns.google",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "8.8.8.8") {
			t.Errorf("path = %q, want to contain 8.8.8.8", r.URL.Path)
		}
		b, _ := json.Marshal(fixture)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	info, err := c.LookupIP(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if info.IP != "8.8.8.8" {
		t.Errorf("IP = %q, want 8.8.8.8", info.IP)
	}
	if info.City != "Mountain View" {
		t.Errorf("City = %q, want Mountain View", info.City)
	}
	if info.Country != "US" {
		t.Errorf("Country = %q, want US", info.Country)
	}
	if info.Org != "AS15169 Google LLC" {
		t.Errorf("Org = %q", info.Org)
	}
	if info.Timezone != "America/Los_Angeles" {
		t.Errorf("Timezone = %q", info.Timezone)
	}
	if info.Hostname != "dns.google" {
		t.Errorf("Hostname = %q", info.Hostname)
	}
	// loc "37.3860,-122.0838" should parse to Lat/Lon
	if info.Lat != "37.3860" {
		t.Errorf("Lat = %q, want 37.3860", info.Lat)
	}
	if info.Lon != "-122.0838" {
		t.Errorf("Lon = %q, want -122.0838", info.Lon)
	}
}

// TestMe checks that the /json endpoint is called for own IP.
func TestMe(t *testing.T) {
	fixture := map[string]any{
		"ip":       "1.2.3.4",
		"city":     "Sydney",
		"timezone": "Australia/Sydney",
		"loc":      "-33.8688,151.2093",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json" {
			t.Errorf("path = %q, want /json", r.URL.Path)
		}
		b, _ := json.Marshal(fixture)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	info, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.IP != "1.2.3.4" {
		t.Errorf("IP = %q, want 1.2.3.4", info.IP)
	}
	if info.Timezone != "Australia/Sydney" {
		t.Errorf("Timezone = %q", info.Timezone)
	}
	if info.Lat != "-33.8688" {
		t.Errorf("Lat = %q, want -33.8688", info.Lat)
	}
	if info.Lon != "151.2093" {
		t.Errorf("Lon = %q, want 151.2093", info.Lon)
	}
}

// TestBogonIP checks that a bogon (private range) response is handled gracefully:
// the IP is set and city/region/loc are empty — no error.
func TestBogonIP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"ip": "192.168.1.1", "bogon": true}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	info, err := c.LookupIP(context.Background(), "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if info.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want 192.168.1.1", info.IP)
	}
	// bogon IPs have no city/loc
	if info.City != "" {
		t.Errorf("City = %q, want empty for bogon", info.City)
	}
	if info.Lat != "" {
		t.Errorf("Lat = %q, want empty for bogon", info.Lat)
	}
	if info.Lon != "" {
		t.Errorf("Lon = %q, want empty for bogon", info.Lon)
	}
}

// TestBatch checks that multiple IPs are looked up and both are returned.
func TestBatch(t *testing.T) {
	var requestCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		ip := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/json")
		resp := map[string]any{"ip": strings.Trim(ip, "/")}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	infos, err := c.BatchLookup(context.Background(), []string{"8.8.8.8", "1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Errorf("len(infos) = %d, want 2", len(infos))
	}
	if requestCount != 2 {
		t.Errorf("requestCount = %d, want 2", requestCount)
	}
}

// TestRetry checks that 429 triggers a retry.
func TestRetry(t *testing.T) {
	var hits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		resp := map[string]any{"ip": "8.8.8.8", "city": "Mountain View", "loc": "37.3860,-122.0838"}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	cfg := ipinfo.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 3
	c := ipinfo.NewClient(cfg)

	info, err := c.LookupIP(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if info.IP != "8.8.8.8" {
		t.Errorf("IP = %q", info.IP)
	}
	if hits < 2 {
		t.Errorf("hits = %d, want at least 2 (retry happened)", hits)
	}
}
