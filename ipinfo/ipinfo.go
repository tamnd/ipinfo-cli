// Package ipinfo is the library behind the ipinfo command line:
// the HTTP client, request shaping, and typed data models for the ipinfo.io
// IP geolocation API (https://ipinfo.io/).
//
// The basic tier (50k lookups/month) requires no API key. The Client paces
// requests, sets a real User-Agent, and retries transient failures (429 and 5xx).
package ipinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to ipinfo.io.
const DefaultUserAgent = "ipinfo-cli/0.1.0 (github.com/tamnd/ipinfo-cli)"

// Host is the API hostname.
const Host = "ipinfo.io"

// BaseURL is the root every API request is built from.
const BaseURL = "https://" + Host

// IPInfo holds geolocation data for an IP address.
type IPInfo struct {
	IP       string `kit:"id" json:"ip"`
	Hostname string `json:"hostname"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Lat      string `json:"lat"`      // first part of loc ("lat,lon")
	Lon      string `json:"lon"`      // second part of loc ("lat,lon")
	Org      string `json:"org"`
	Postal   string `json:"postal"`
	Timezone string `json:"timezone"`
}

// Config holds all tunable parameters for the Client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to the ipinfo.io API.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// LookupIP fetches geolocation info for a specific IP address.
func (c *Client) LookupIP(ctx context.Context, ip string) (*IPInfo, error) {
	rawURL := fmt.Sprintf("%s/%s/json", c.cfg.BaseURL, ip)
	return c.fetch(ctx, rawURL)
}

// Me fetches geolocation info for the caller's own IP address.
func (c *Client) Me(ctx context.Context) (*IPInfo, error) {
	rawURL := c.cfg.BaseURL + "/json"
	return c.fetch(ctx, rawURL)
}

// BatchLookup fetches geolocation info for multiple IPs, sequentially.
func (c *Client) BatchLookup(ctx context.Context, ips []string) ([]*IPInfo, error) {
	out := make([]*IPInfo, 0, len(ips))
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		info, err := c.LookupIP(ctx, ip)
		if err != nil {
			return nil, fmt.Errorf("lookup %s: %w", ip, err)
		}
		out = append(out, info)
	}
	return out, nil
}

// wire is the raw JSON shape returned by the API, including the "loc" field
// and the optional "bogon" flag.
type wire struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Loc      string `json:"loc"`
	Org      string `json:"org"`
	Postal   string `json:"postal"`
	Timezone string `json:"timezone"`
	Bogon    bool   `json:"bogon"`
}

func (c *Client) fetch(ctx context.Context, rawURL string) (*IPInfo, error) {
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var w wire
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("parse ipinfo: %w", err)
	}
	info := &IPInfo{
		IP:       w.IP,
		Hostname: w.Hostname,
		City:     w.City,
		Region:   w.Region,
		Country:  w.Country,
		Org:      w.Org,
		Postal:   w.Postal,
		Timezone: w.Timezone,
	}
	// Parse "lat,lon" from loc; bogon IPs may have empty loc — that's fine.
	if w.Loc != "" {
		parts := strings.SplitN(w.Loc, ",", 2)
		if len(parts) == 2 {
			info.Lat = parts[0]
			info.Lon = parts[1]
		}
	}
	return info, nil
}

// --- internal helpers ---

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
