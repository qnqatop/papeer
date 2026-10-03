package download

import (
	"errors"
	"testing"
	"time"
)

func TestRateGate_TripAndExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	g := NewRateGate()
	g.now = func() time.Time { return now }

	if _, blocked := g.Blocked("api.semanticscholar.org"); blocked {
		t.Fatal("fresh gate must not block")
	}

	g.Trip("api.semanticscholar.org", 90*time.Second)
	until, blocked := g.Blocked("api.semanticscholar.org")
	if !blocked || !until.Equal(now.Add(90*time.Second)) {
		t.Fatalf("Blocked = %v, %v; want true until +90s", until, blocked)
	}
	if _, blocked := g.Blocked("api.openalex.org"); blocked {
		t.Error("other hosts must not be blocked")
	}

	now = now.Add(90 * time.Second)
	if _, blocked := g.Blocked("api.semanticscholar.org"); blocked {
		t.Error("gate must reopen once the block expires")
	}
}

func TestRateGate_MinimumAndLongerBlockKept(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	g := NewRateGate()
	g.now = func() time.Time { return now }

	g.Trip("h", 0) // no Retry-After → MinRateTrip
	if until, _ := g.Blocked("h"); !until.Equal(now.Add(MinRateTrip)) {
		t.Errorf("until = %v, want now+MinRateTrip", until)
	}

	g.Trip("h", 5*time.Minute)
	g.Trip("h", 10*time.Second) // shorter trip must not shorten the block
	if until, _ := g.Blocked("h"); !until.Equal(now.Add(5 * time.Minute)) {
		t.Errorf("until = %v, want now+5m", until)
	}
}

func TestRateGate_NilIsOpen(t *testing.T) {
	var g *RateGate
	g.Trip("h", time.Hour)
	if _, blocked := g.Blocked("h"); blocked {
		t.Error("nil gate must never block")
	}
}

func TestLookups_CachesValuesAndErrors(t *testing.T) {
	l := NewLookups()
	calls := 0
	boom := errors.New("boom")
	fetch := func() (any, error) { calls++; return nil, boom }

	for range 3 {
		if _, err := l.Do("k", fetch); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want boom", err)
		}
	}
	if calls != 1 {
		t.Errorf("fetch called %d times, want 1", calls)
	}
	if r, ok := l.Peek("k"); !ok || !errors.Is(r.Err, boom) {
		t.Errorf("Peek = %+v, %v", r, ok)
	}
	if _, ok := l.Peek("other"); ok {
		t.Error("Peek of unknown key must miss")
	}

	var nilMemo *Lookups
	nilCalls := 0
	for range 2 {
		_, _ = nilMemo.Do("k", func() (any, error) { nilCalls++; return 1, nil })
	}
	if nilCalls != 2 {
		t.Errorf("nil memo must not cache: calls = %d", nilCalls)
	}
}
