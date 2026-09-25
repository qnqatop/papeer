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
	// Russian names are written surname first ("Иванов И. И.",
	// "Иванов Иван Иванович"), so the last word is an initial or a patronymic.
	if hasCyrillic(name) {
		lastName = parts[0]
	}
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

// cyrillicTranslit maps lowercase Russian letters to Latin (a simplified
// GOST 7.79-2000 system B / ISO 9 style, the kind used for article URLs).
var cyrillicTranslit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	// Ukrainian/Belarusian letters that also show up on CyberLeninka.
	'і': "i", 'ї': "yi", 'є': "ye", 'ґ': "g", 'ў': "u",
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// transliterateCyrillic replaces Cyrillic letters with their Latin spelling;
// other characters pass through unchanged. It must run before the NFD step in
// toASCII, which would otherwise decompose "й" into "и" + a combining mark.
func transliterateCyrillic(s string) string {
	if !hasCyrillic(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		lr := unicode.ToLower(r)
		if lat, ok := cyrillicTranslit[lr]; ok {
			if r != lr && lat != "" {
				// Keep the capital so the result reads naturally; callers
				// lowercase anyway.
				lat = strings.ToUpper(lat[:1]) + lat[1:]
			}
			b.WriteString(lat)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// toASCII transliterates Unicode to ASCII: Cyrillic is transliterated to Latin,
// diacritical marks are stripped, and any remaining non-ASCII characters are
// dropped.
func toASCII(s string) string {
	s = transliterateCyrillic(s)
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
