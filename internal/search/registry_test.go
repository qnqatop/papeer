package search

import (
	"testing"

	"github.com/qnqatop/papeer/internal/httpclient"
)

func TestSpecs_MatchProviders(t *testing.T) {
	c := httpclient.New("test@example.com")
	seen := map[string]bool{}
	for _, s := range Specs() {
		if seen[s.Name] {
			t.Errorf("duplicate spec %q", s.Name)
		}
		seen[s.Name] = true
		if s.ProbeURL == "" || s.New == nil {
			t.Errorf("%s: incomplete spec", s.Name)
			continue
		}
		p := s.New(c, "test@example.com")
		if p.Name() != s.Name {
			t.Errorf("spec %q builds provider named %q", s.Name, p.Name())
		}
		if p.Language() != s.Lang {
			t.Errorf("spec %q: Lang = %q, provider Language() = %q", s.Name, s.Lang, p.Language())
		}
	}
}

func TestNewProviders_OnePerSpec(t *testing.T) {
	c := httpclient.New("test@example.com")
	specs := Specs()
	providers := NewProviders(c, "test@example.com")
	if len(providers) != len(specs) {
		t.Fatalf("got %d providers, want %d", len(providers), len(specs))
	}
	for i, p := range providers {
		if p.Name() != specs[i].Name {
			t.Errorf("provider %d = %q, want %q (order must follow Specs)", i, p.Name(), specs[i].Name)
		}
	}
}
