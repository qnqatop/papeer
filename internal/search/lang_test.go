package search

import "testing"

func TestDetectLang(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Рекомендательная система для абитуриентов", "ru"},
		{"A Survey of Recommender Systems", "en"},
		{"", ""},
		{"2024 :： — 123", ""},
		// Mixed: majority script wins.
		{"Рекомендательные системы (recommender systems)", "ru"},
		{"Recommender systems: рекомендации", "en"},
	}
	for _, c := range cases {
		if got := detectLang(c.in); got != c.want {
			t.Errorf("detectLang(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMatchesLangScope_FallsBackToAbstract(t *testing.T) {
	// Title has no letters → judge by abstract.
	p := RawPaper{Title: "2024", Abstract: "Русскоязычная аннотация статьи"}
	if !matchesLangScope(p, "ru") {
		t.Error("expected ru match via abstract fallback")
	}
	if matchesLangScope(p, "en") {
		t.Error("did not expect en match for Russian abstract")
	}
}

func TestMatchesLangScope_KeepsUndetectable(t *testing.T) {
	p := RawPaper{Title: "123 — 456", Abstract: ""}
	if !matchesLangScope(p, "ru") || !matchesLangScope(p, "en") {
		t.Error("undetectable-language paper should be kept for any scope")
	}
}

func TestFilterByLangScope(t *testing.T) {
	papers := []RawPaper{
		{Title: "Рекомендательная система"},
		{Title: "English Recommender Paper"},
		{Title: "Ещё одна русская статья"},
	}
	ru := filterByLangScope(papers, "ru")
	if len(ru) != 2 {
		t.Fatalf("ru filter kept %d, want 2", len(ru))
	}
	en := filterByLangScope(papers, "en")
	if len(en) != 1 || en[0].Title != "English Recommender Paper" {
		t.Fatalf("en filter = %+v, want the single English paper", en)
	}
	// Input must not be mutated.
	if len(papers) != 3 {
		t.Errorf("input slice was mutated: len = %d", len(papers))
	}
}
