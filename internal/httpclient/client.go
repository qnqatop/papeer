package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// StatusError is returned when an HTTP request completes with a non-2xx status.
// It lets callers match on the numeric code (via errors.As) instead of parsing
// the error string.
type StatusError struct {
	Code int
	Host string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d from %s", e.Code, e.Host)
}

// newCookieJar returns an in-memory cookie jar. We use one jar per Client so
// publisher sites (MDPI, Springer) can set the "I've seen you" cookie on the
// landing page and reuse it on the PDF fetch. nil cookies on jar failure —
// caller still works, just without cross-request cookies.
func newCookieJar() http.CookieJar {
	jar, _ := cookiejar.New(nil)
	return jar
}

// User-Agent strings — realistic browser UAs for bypassing basic bot detection.
var (
	UAFirefox = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:120.0) Gecko/20100101 Firefox/120.0"
	UAChrome  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	UALinux   = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
)

// PoliteUA builds a script-like UA with contact email.
func PoliteUA(email string) string {
	if email == "" {
		return "papeer/1.0 (academic research)"
	}
	return fmt.Sprintf("papeer/1.0 (academic research; mailto:%s)", email)
}

// Client wraps http.Client with rate limiting and UA strategies.
type Client struct {
	http        *http.Client
	rateReg     *RateLimitRegistry
	email       string
	hostHeaders map[string]map[string]string // host → header name → value
}

// SetHostHeader registers an extra header that DoJSON/DoText will send when
// the request host matches. Used for API keys (e.g. Semantic Scholar x-api-key).
// Safe to call before the first request; not safe to call concurrently with
// in-flight requests.
func (c *Client) SetHostHeader(host, name, value string) {
	if c.hostHeaders == nil {
		c.hostHeaders = make(map[string]map[string]string)
	}
	if c.hostHeaders[host] == nil {
		c.hostHeaders[host] = make(map[string]string)
	}
	c.hostHeaders[host][name] = value
}

// SetHostRateLimit overrides the per-host rate limit (requests/sec). Call
// this when a host-specific API key unlocks a higher tier than the polite
// default (e.g. Semantic Scholar: ~0.33 req/s unauthenticated vs ~1 req/s
// with an x-api-key) — without this the client would keep throttling at the
// slow default even though the key would allow faster requests.
func (c *Client) SetHostRateLimit(host string, reqPerSec float64) {
	c.rateReg.SetLimit(host, rate.Limit(reqPerSec))
}

func (c *Client) applyHostHeaders(req *http.Request) {
	if c.hostHeaders == nil {
		return
	}
	if headers, ok := c.hostHeaders[req.URL.Host]; ok {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
}

// New creates a Client with the given email for polite pool APIs.
func New(email string) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 60 * time.Second,
			Jar:     newCookieJar(),
		},
		rateReg: NewRateLimitRegistry(),
		email:   email,
	}
}

// NewWithProxy creates a Client that routes all requests through the given proxy.
// proxyURL should be like "http://host:port" or "socks5://host:port".
func NewWithProxy(email, proxyURL string) (*Client, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}
	transport := &http.Transport{
		Proxy: http.ProxyURL(parsed),
	}
	return &Client{
		http: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
			Jar:       newCookieJar(),
		},
		rateReg: NewRateLimitRegistry(),
		email:   email,
	}, nil
}

// NewWithRegistry creates a Client with a shared rate limit registry.
func NewWithRegistry(email string, reg *RateLimitRegistry) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 60 * time.Second,
			Jar:     newCookieJar(),
		},
		rateReg: reg,
		email:   email,
	}
}

// DoJSON makes a GET request expecting JSON, with polite UA and rate limiting.
func (c *Client) DoJSON(ctx context.Context, rawURL string, result interface{}) error {
	host := hostFromURL(rawURL)
	if err := c.rateReg.Wait(ctx, host); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", PoliteUA(c.email))
	req.Header.Set("Accept", "application/json")
	c.applyHostHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &StatusError{Code: resp.StatusCode, Host: host}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

// DoJSONWithRetry retries on 429/5xx errors with exponential backoff.
func (c *Client) DoJSONWithRetry(ctx context.Context, rawURL string, result interface{}, maxRetries int) error {
	var lastErr error
	backoff := []time.Duration{3 * time.Second, 8 * time.Second, 20 * time.Second}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		lastErr = c.DoJSON(ctx, rawURL, result)
		if lastErr == nil {
			return nil
		}

		// Only retry on rate limit or server errors.
		if attempt < maxRetries && isRetryable(lastErr) {
			delay := backoff[min(attempt, len(backoff)-1)]
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		break
	}
	return lastErr
}

// DoText makes a GET request expecting text (e.g., arXiv Atom XML).
func (c *Client) DoText(ctx context.Context, rawURL string) (string, error) {
	host := hostFromURL(rawURL)
	if err := c.rateReg.Wait(ctx, host); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", PoliteUA(c.email))
	c.applyHostHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", &StatusError{Code: resp.StatusCode, Host: host}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// PostJSON sends a JSON POST and returns the raw response body. Used for
// out-of-band publisher challenges (e.g. Akamai's /_sec/verify endpoint) that
// don't fit the regular search/download flow. Cookies set on the response are
// stored in the client's jar so subsequent GETs benefit.
func (c *Client) PostJSON(ctx context.Context, rawURL string, body []byte, extraHeaders map[string]string) ([]byte, int, error) {
	host := hostFromURL(rawURL)
	if err := c.rateReg.Wait(ctx, host); err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", UAFirefox)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	// Apply per-host headers (e.g. Semantic Scholar x-api-key) last so batch
	// API calls authenticate the same way GET requests do via DoJSON.
	c.applyHostHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return rb, resp.StatusCode, nil
}

// DownloadFile downloads a URL with UA rotation strategy (3 attempts on 403).
// Returns response body, content-type, and error.
func (c *Client) DownloadFile(ctx context.Context, rawURL string) ([]byte, string, error) {
	return c.downloadFile(ctx, rawURL, "")
}

// DownloadFileWithReferer is like DownloadFile but sends the given Referer on
// every attempt. Some hosts (e.g. CyberLeninka) serve a captcha/HTML instead of
// the PDF unless the request carries a Referer pointing at the article page —
// the default per-host Referer used on retries is not enough. An empty referer
// falls back to the standard DownloadFile behavior.
func (c *Client) DownloadFileWithReferer(ctx context.Context, rawURL, referer string) ([]byte, string, error) {
	return c.downloadFile(ctx, rawURL, referer)
}

// downloadFile is the shared implementation. When referer is non-empty it is
// applied to all attempts; otherwise the polite first attempt sends no Referer
// and the UA-rotation retries fall back to "https://<host>/".
func (c *Client) downloadFile(ctx context.Context, rawURL, referer string) ([]byte, string, error) {
	type uaStrategy struct {
		ua      string
		referer string
	}

	host := hostFromURL(rawURL)
	strategies := []uaStrategy{
		{ua: PoliteUA(c.email)},
		{ua: UAFirefox, referer: fmt.Sprintf("https://%s/", host)},
		{ua: UAChrome, referer: fmt.Sprintf("https://%s/", host)},
	}
	// An explicit referer overrides the default on every attempt.
	if referer != "" {
		for i := range strategies {
			strategies[i].referer = referer
		}
	}

	var lastErr error
	for _, s := range strategies {
		if err := c.rateReg.WaitDownload(ctx); err != nil {
			return nil, "", err
		}

		req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", s.ua)
		req.Header.Set("Accept", "application/pdf, text/html;q=0.9, */*;q=0.5")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,ru;q=0.8")
		// Browser-like sec-fetch headers — publishers (MDPI, Springer) check
		// these to distinguish curl-style scripts from real browsers.
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-User", "?1")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("DNT", "1")
		if s.referer != "" {
			req.Header.Set("Referer", s.referer)
		}

		// Use a longer timeout for file downloads.
		dlClient := *c.http
		dlClient.Timeout = 90 * time.Second

		resp, err := dlClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == 403 {
			lastErr = &StatusError{Code: 403, Host: host}
			continue // try next UA
		}
		if resp.StatusCode >= 400 {
			return nil, "", &StatusError{Code: resp.StatusCode, Host: host}
		}

		ct := resp.Header.Get("Content-Type")
		return body, ct, nil
	}

	return nil, "", fmt.Errorf("all UA strategies failed: %w", lastErr)
}

func hostFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown"
	}
	return u.Host
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "429") || strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") || strings.Contains(msg, "504")
}
