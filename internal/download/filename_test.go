package download

import (
	"strings"
	"testing"
)

func TestGenerateFilename_Normal(t *testing.T) {
	got := GenerateFilename("John Smith", 2023, "Cold Start in Recommender Systems")
	// Expect: smith_2023_cold_start_in_recommender_systems_{hash}.pdf
	if !strings.HasPrefix(got, "smith_2023_cold_start_in_recommender_systems_") {
		t.Errorf("unexpected prefix: %s", got)
	}
	if !strings.HasSuffix(got, ".pdf") {
		t.Errorf("expected .pdf suffix: %s", got)
	}
	// Hash part should be 8 hex chars.
	parts := strings.Split(strings.TrimSuffix(got, ".pdf"), "_")
	hash := parts[len(parts)-1]
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash, got %q (len %d)", hash, len(hash))
	}
}

func TestGenerateFilename_NoAuthor(t *testing.T) {
	got := GenerateFilename("", 2023, "Some Title")
	if !strings.HasPrefix(got, "unknown_2023_") {
		t.Errorf("expected 'unknown' author: %s", got)
	}
}

func TestGenerateFilename_NoYear(t *testing.T) {
	got := GenerateFilename("Alice", 0, "Some Title")
	if !strings.Contains(got, "_0000_") {
		t.Errorf("expected year 0000: %s", got)
	}
}

func TestGenerateFilename_LongTitle(t *testing.T) {
	longTitle := strings.Repeat("word ", 100) // 500 chars
	got := GenerateFilename("Author", 2020, longTitle)

	// Remove author_year_ prefix and _hash.pdf suffix to isolate slug.
	withoutExt := strings.TrimSuffix(got, ".pdf")
	// Find slug: after "author_2020_" and before the last "_hash".
	prefix := "author_2020_"
	rest := strings.TrimPrefix(withoutExt, prefix)
	lastUnderscore := strings.LastIndex(rest, "_")
	slug := rest[:lastUnderscore]

	if len(slug) > 50 {
		t.Errorf("slug too long (%d chars): %q", len(slug), slug)
	}
}

func TestGenerateFilename_SpecialChars(t *testing.T) {
	got := GenerateFilename("José García", 2021, "Ünïcödé & Spëcìal—Chars!")
	if !strings.HasPrefix(got, "garcia_2021_") {
		t.Errorf("expected transliterated author 'garcia': %s", got)
	}
	// Slug should contain only lowercase alphanumeric and underscores.
	withoutExt := strings.TrimSuffix(got, ".pdf")
	for _, r := range withoutExt {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.') {
			t.Errorf("unexpected char %q in filename: %s", string(r), got)
		}
	}
}

func TestGenerateFilename_Uniqueness(t *testing.T) {
	f1 := GenerateFilename("Smith", 2023, "Title Alpha")
	f2 := GenerateFilename("Smith", 2023, "Title Beta")
	if f1 == f2 {
		t.Errorf("expected different filenames for different titles, got same: %s", f1)
	}
}
