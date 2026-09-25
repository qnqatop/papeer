package httpclient

import (
	"context"
	"testing"
	"time"
)

func TestRateLimitRegistry_KnownHost(t *testing.T) {
	reg := NewRateLimitRegistry()
	ctx := context.Background()

	// arXiv is ~0.3 req/s = ~3.3s between requests.
	// First call should pass immediately.
	start := time.Now()
	if err := reg.Wait(ctx, "export.arxiv.org"); err != nil {
		t.Fatal(err)
	}
	first := time.Since(start)
	if first > 100*time.Millisecond {
		t.Errorf("first call took %v, expected near-instant", first)
	}

	// Second call should wait ~3s.
	start = time.Now()
	if err := reg.Wait(ctx, "export.arxiv.org"); err != nil {
		t.Fatal(err)
	}
	second := time.Since(start)
	if second < 2*time.Second {
		t.Errorf("second call took %v, expected >=2s for arXiv rate limit", second)
	}
}

func TestRateLimitRegistry_UnknownHost(t *testing.T) {
	reg := NewRateLimitRegistry()
	ctx := context.Background()

	// Unknown host should use default (5 req/s) — near-instant.
	start := time.Now()
	if err := reg.Wait(ctx, "unknown.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Wait(ctx, "unknown.example.com"); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("two calls to unknown host took %v, expected <500ms", elapsed)
	}
}

func TestRateLimitRegistry_SetLimit(t *testing.T) {
	reg := NewRateLimitRegistry()
	ctx := context.Background()

	// Override a strict default (arXiv, ~0.3 req/s) with a fast limit — both
	// calls should now go through near-instantly instead of waiting ~3s.
	reg.SetLimit("export.arxiv.org", 1000)

	start := time.Now()
	if err := reg.Wait(ctx, "export.arxiv.org"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Wait(ctx, "export.arxiv.org"); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("two calls after SetLimit(1000) took %v, expected near-instant", elapsed)
	}
}

func TestRateLimitRegistry_SetLimit_ReplacesPreviouslyCreatedLimiter(t *testing.T) {
	reg := NewRateLimitRegistry()
	ctx := context.Background()

	// First touch an unknown host (creates a default 5 req/s limiter)...
	if err := reg.Wait(ctx, "custom.example.com"); err != nil {
		t.Fatal(err)
	}
	// ...then override it with a much slower limit. SetLimit installs a
	// brand-new limiter (fresh burst token included), so the immediately
	// following call succeeds, but the one after that must block ~1s at the
	// new 1 req/s rate — proving the override actually took effect rather
	// than being ignored in favor of the original 5 req/s default.
	reg.SetLimit("custom.example.com", 1) // 1 req/s

	if err := reg.Wait(ctx, "custom.example.com"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := reg.Wait(ctx, "custom.example.com"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Errorf("expected SetLimit override (1 req/s) to slow down the second wait")
	}
}

func TestRateLimitRegistry_ContextCancel(t *testing.T) {
	reg := NewRateLimitRegistry()
	ctx, cancel := context.WithCancel(context.Background())

	// Exhaust the token.
	reg.Wait(ctx, "export.arxiv.org")

	// Cancel context — next Wait should return error quickly.
	cancel()
	err := reg.Wait(ctx, "export.arxiv.org")
	if err == nil {
		t.Error("expected error on cancelled context")
	}
}

func TestRateLimitRegistry_SetLimit_SameLimitKeepsLimiter(t *testing.T) {
	reg := NewRateLimitRegistry()
	reg.SetLimit("slow.example", 0.5)
	ctx := context.Background()
	if err := reg.Wait(ctx, "slow.example"); err != nil {
		t.Fatal(err)
	}
	// A shared registry is re-configured by every new client; re-setting the
	// same limit must not hand out a fresh token.
	reg.SetLimit("slow.example", 0.5)
	start := time.Now()
	if err := reg.Wait(ctx, "slow.example"); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < time.Second {
		t.Errorf("second Wait took %v, want ~2s (limiter must not be reset)", waited)
	}
}
