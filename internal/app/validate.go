package app

import (
	"errors"
	"regexp"
	"strings"
)

// emailRe is a pragmatic email shape check.
// Not RFC-5322 strict; rejects obvious garbage and example domains.
var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

// ErrInvalidEmail is returned when a profile is missing a valid contact email.
// Email is required because Unpaywall/OpenAlex/Crossref give meaningful rate
// limits and OA resolution only to identified clients.
var ErrInvalidEmail = errors.New("a real contact email is required for the profile " +
	"(used by Unpaywall, OpenAlex and Crossref polite pools)")

// IsValidEmail reports whether s is a syntactically valid contact email
// suitable for the polite API pool. Empty strings, whitespace-only strings,
// and example/test placeholders are rejected.
func IsValidEmail(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return false
	}
	if !emailRe.MatchString(s) {
		return false
	}
	// Reject common placeholder domains.
	host := s[strings.Index(s, "@")+1:]
	for _, bad := range []string{"example.com", "example.org", "example.net", "test.com", "localhost"} {
		if host == bad {
			return false
		}
	}
	return true
}
