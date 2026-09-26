package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Body size limits. API responses are small JSON/XML documents; PDFs can be
// large but anything past MaxDownloadSize is almost certainly not a paper.
const (
	MaxAPIBodySize  int64 = 20 << 20  // 20 MiB
	MaxDownloadSize int64 = 200 << 20 // 200 MiB

	maxRedirects  = 10
	maxRetryAfter = 60 * time.Second
)

// ErrBodyTooLarge is returned when a response body exceeds the configured limit.
var ErrBodyTooLarge = errors.New("response body too large")

// ErrBlockedDestination is returned by a client with the SSRF guard enabled
// when a request targets a non-public address or a non-http(s) scheme.
var ErrBlockedDestination = errors.New("destination not allowed")

// readLimited reads at most limit bytes from resp.Body. It rejects early on
// a declared Content-Length above the limit and otherwise reads limit+1 bytes
// to detect bodies that lie about (or omit) their length.
func readLimited(resp *http.Response, limit int64) ([]byte, error) {
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("%w: %d bytes (limit %d)", ErrBodyTooLarge, resp.ContentLength, limit)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrBodyTooLarge, limit)
	}
	return body, nil
}

// IsDisallowedIP reports whether ip is an address outbound requests must not
// reach: loopback, private (RFC 1918, fc00::/7), link-local (incl. the cloud
// metadata endpoint 169.254.169.254), unspecified or multicast.
func IsDisallowedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsValid() ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// IsDisallowedHost reports whether host (without port) is "localhost" or a
// literal IP that IsDisallowedIP rejects. Hostnames are not resolved — the
// dial-time check covers those.
func IsDisallowedHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if h == "" || h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if i := strings.IndexByte(h, '%'); i >= 0 { // IPv6 zone
		h = h[:i]
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		return IsDisallowedIP(ip)
	}
	return false
}

// checkURL validates the scheme and literal host of a request URL.
func checkURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q", ErrBlockedDestination, u.Scheme)
	}
	if IsDisallowedHost(u.Hostname()) {
		return fmt.Errorf("%w: host %q", ErrBlockedDestination, u.Hostname())
	}
	return nil
}

// ssrfDialControl rejects connections to disallowed IPs. It runs after DNS
// resolution, so a hostname that resolves (or re-resolves) to a private
// address is refused too.
func ssrfDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBlockedDestination, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || IsDisallowedIP(ip) {
		return fmt.Errorf("%w: %s", ErrBlockedDestination, host)
	}
	return nil
}

// guardTransport checks every outgoing request (including redirect hops)
// before handing it to the underlying transport.
type guardTransport struct {
	base http.RoundTripper
}

func (t *guardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkURL(req.URL); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// EnableSSRFGuard restricts the client to public http(s) destinations.
// Without a proxy, the resolved IP is checked at dial time (defeating DNS
// rebinding). With a proxy the dial check is skipped — the proxy itself may
// be on localhost — but literal-IP and localhost target URLs are still
// rejected before sending. Opt-in so tests can talk to httptest servers.
// Call before the first request.
func (c *Client) EnableSSRFGuard() {
	var base *http.Transport
	switch t := c.http.Transport.(type) {
	case *http.Transport:
		base = t.Clone()
	case nil:
		base = http.DefaultTransport.(*http.Transport).Clone()
	default:
		c.http.Transport = &guardTransport{base: t}
		return
	}
	if base.Proxy == nil {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   ssrfDialControl,
		}
		base.DialContext = dialer.DialContext
	}
	c.http.Transport = &guardTransport{base: base}
}

// checkRedirect limits redirect chains to http(s), caps the hop count, and
// strips per-host custom headers (e.g. API keys registered via SetHostHeader)
// once the chain leaves the original host. net/http itself only strips
// Authorization and Cookie headers.
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("redirect to unsupported scheme %q", req.URL.Scheme)
	}
	orig := via[0].URL.Host
	if req.URL.Host != orig {
		for name := range c.hostHeaders[orig] {
			req.Header.Del(name)
		}
	}
	return nil
}

// parseRetryAfter parses the delay-seconds form of Retry-After, capped at
// maxRetryAfter. Returns 0 when absent or not in seconds form.
func parseRetryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	return d
}

// IsTransient reports whether err is worth retrying: 408/429/5xx gateway
// statuses, truncated responses, connection resets and timeouts. Caller
// cancellation (context.Canceled) is never transient. Matching is on error
// types, not text, so URLs containing "503" or "eof" don't trip it.
func IsTransient(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var se *StatusError
	if errors.As(err, &se) {
		switch se.Code {
		case 408, 429, 500, 502, 503, 504:
			return true
		}
		return false
	}
	if errors.Is(err, ErrBlockedDestination) || errors.Is(err, ErrBodyTooLarge) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
