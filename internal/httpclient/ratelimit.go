package httpclient

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimitRegistry manages per-host rate limiters.
type RateLimitRegistry struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	defaults map[string]rate.Limit // pre-configured hosts
}

// NewRateLimitRegistry creates a registry with known API rate limits.
// Limits are derived from each API's publicly stated rate limits.
func NewRateLimitRegistry() *RateLimitRegistry {
	return &RateLimitRegistry{
		limiters: make(map[string]*rate.Limiter),
		defaults: map[string]rate.Limit{
			"api.semanticscholar.org": rate.Limit(0.33), // 100 req/5min without API key (~1 per 3s)
			"api.openalex.org":        rate.Limit(9),    // 10 req/s polite pool
			"api.crossref.org":        rate.Limit(5),    // generous with mailto
			"export.arxiv.org":        rate.Limit(0.3),  // 1 req/3s — arXiv is strict
			"api.unpaywall.org":       rate.Limit(9),    // 100k/day, effectively high
			"_download":               rate.Limit(3),    // general PDF downloads
		},
	}
}

// SetLimit overrides (or pre-creates) the rate limiter for a host, replacing
// whatever default or previously-set limiter was in place. Used when a paid
// or keyed API tier unlocks a materially higher rate than the polite-pool
// default (e.g. Semantic Scholar with an x-api-key).
func (r *RateLimitRegistry) SetLimit(host string, limit rate.Limit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limiters[host] = rate.NewLimiter(limit, 1)
}

// Wait blocks until the rate limit for the given host allows a request.
func (r *RateLimitRegistry) Wait(ctx context.Context, host string) error {
	r.mu.Lock()
	lim, ok := r.limiters[host]
	if !ok {
		limit, exists := r.defaults[host]
		if !exists {
			limit = rate.Limit(5) // default: 5 req/s for unknown hosts
		}
		lim = rate.NewLimiter(limit, 1)
		r.limiters[host] = lim
	}
	r.mu.Unlock()

	return lim.Wait(ctx)
}

// WaitDownload blocks using the general download rate limit.
func (r *RateLimitRegistry) WaitDownload(ctx context.Context) error {
	return r.Wait(ctx, "_download")
}
