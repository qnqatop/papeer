package download

import (
	"fmt"
	"sync"
	"time"
)

// MinRateTrip is the shortest time a host stays blocked after it answered
// 429. Semantic Scholar's unauthenticated pool usually stays throttled for
// longer than its Retry-After hint, so short hints are rounded up.
const MinRateTrip = 60 * time.Second

// RateGate is a per-host circuit breaker shared by download engines. Once a
// metadata API rate-limits us, sources skip it until the block expires
// instead of spending their retries (and the user's time) on more 429s. It is
// app-scoped so consecutive single-paper downloads see the same state. A nil
// *RateGate never blocks.
type RateGate struct {
	mu    sync.Mutex
	until map[string]time.Time
	now   func() time.Time // test hook
}

// NewRateGate returns an empty gate.
func NewRateGate() *RateGate {
	return &RateGate{until: make(map[string]time.Time), now: time.Now}
}

// Blocked reports whether host is currently tripped and, if so, until when.
func (g *RateGate) Blocked(host string) (time.Time, bool) {
	if g == nil {
		return time.Time{}, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.until[host]
	if !ok {
		return time.Time{}, false
	}
	if !g.now().Before(until) {
		delete(g.until, host)
		return time.Time{}, false
	}
	return until, true
}

// Trip blocks host for max(retryAfter, MinRateTrip). An existing longer block
// is kept.
func (g *RateGate) Trip(host string, retryAfter time.Duration) {
	if g == nil {
		return
	}
	retryAfter = max(retryAfter, MinRateTrip)
	g.mu.Lock()
	defer g.mu.Unlock()
	until := g.now().Add(retryAfter)
	if prev, ok := g.until[host]; ok && prev.After(until) {
		return
	}
	g.until[host] = until
}

// RateLimitedError is returned by a source helper that skipped a request
// because the host is tripped in the RateGate.
type RateLimitedError struct {
	Host  string
	Until time.Time
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("%s rate limited (retry after %s)", e.Host, e.Until.Format("15:04:05"))
}

// LookupResult is a memoized metadata response (or the error it ended with).
type LookupResult struct {
	Val any
	Err error
}

// Lookups memoizes metadata API responses for a single DownloadOne call, so
// several sources in the chain that need the same record (e.g. the S2 paper
// by DOI for both arxiv and s2_doi) share one request. A nil *Lookups does
// not cache.
type Lookups struct {
	mu      sync.Mutex
	entries map[string]LookupResult
}

// NewLookups returns an empty memo.
func NewLookups() *Lookups {
	return &Lookups{entries: make(map[string]LookupResult)}
}

// Do returns the cached result for key, or runs fetch and caches its result,
// errors included. The lock is held during fetch so concurrent callers for
// the same paper never issue duplicate requests.
func (l *Lookups) Do(key string, fetch func() (any, error)) (any, error) {
	if l == nil {
		return fetch()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.entries[key]; ok {
		return r.Val, r.Err
	}
	v, err := fetch()
	l.entries[key] = LookupResult{Val: v, Err: err}
	return v, err
}

// Peek returns the cached result for key without fetching.
func (l *Lookups) Peek(key string) (LookupResult, bool) {
	if l == nil {
		return LookupResult{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.entries[key]
	return r, ok
}
