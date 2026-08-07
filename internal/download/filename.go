package download

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	reNonAlnum       = regexp.MustCompile(`[^a-z0-9]+`)
	reTrailingUndesc = regexp.MustCompile(`_+$`)
)

// GenerateFilename builds a deterministic PDF filename from paper metadata.
// Format: {author}_{year}_{slug}_{hash}.pdf
func GenerateFilename(firstAuthor string, year int, title string) string {
	author := normalizeAuthor(firstAuthor)
	yearStr := fmt.Sprintf("%04d", year)
	slug := buildSlug(title)
	hash := titleHash(title)

	return fmt.Sprintf("%s_%s_%s_%s.pdf", author, yearStr, slug, hash)
}

// normalizeAuthor extracts the last name (last word), transliterates to ASCII,
// and lowercases it. Returns "unknown" if empty.
func normalizeAuthor(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unknown"
	}

	parts := strings.Fields(name)
	lastName := parts[len(parts)-1]
	lastName = toASCII(lastName)
	lastName = strings.ToLower(lastName)
	// Remove any remaining non-alpha characters.
	lastName = reNonAlnum.ReplaceAllString(lastName, "")
	if lastName == "" {
		return "unknown"
	}
	return lastName
}

// buildSlug creates a URL-style slug from the title: lowercase, ASCII-only,
// non-alphanumeric replaced with underscores, collapsed, trimmed, max 50 chars.
func buildSlug(title string) string {
	s := toASCII(title)
	s = strings.ToLower(s)
	s = reNonAlnum.ReplaceAllString(s, "_")
	s = reTrailingUndesc.ReplaceAllString(s, "")

	// Also trim leading underscores.
	s = strings.TrimLeft(s, "_")

	if len(s) > 50 {
		s = s[:50]
		// Re-trim trailing underscores after truncation.
		s = reTrailingUndesc.ReplaceAllString(s, "")
	}

	if s == "" {
		s = "untitled"
	}
	return s
}

// titleHash returns the first 8 hex chars of SHA-256 of the original title.
func titleHash(title string) string {
	h := sha256.Sum256([]byte(title))
	return hex.EncodeToString(h[:])[:8]
}

// toASCII transliterates Unicode to ASCII by stripping diacritical marks
// and dropping any remaining non-ASCII characters.
func toASCII(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)

	// Drop anything still non-ASCII.
	var b strings.Builder
	for _, r := range result {
		if r < 128 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
