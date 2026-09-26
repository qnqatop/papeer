package app

import (
	"errors"
	"fmt"
	"net"
	"net/url"
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

// LLM base URL errors.
var (
	ErrInsecureLLMBaseURL = errors.New("LLM base URL must use https (plain http is allowed only for localhost)")
	ErrLLMKeyReentry      = errors.New("the LLM base URL host changed: re-enter the API key " +
		"so the stored key is not sent to the new host")
)

// defaultLLMOrigin is where the OpenAI SDK sends requests for an empty base URL.
const defaultLLMOrigin = "https://api.openai.com:443"

// validateLLMBaseURL requires an https base URL, except for loopback hosts
// (local models such as Ollama/LM Studio), so API keys never travel in clear
// text over the network. Empty means the SDK default (https).
func validateLLMBaseURL(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid LLM base URL %q", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return ErrInsecureLLMBaseURL
	default:
		return fmt.Errorf("invalid LLM base URL %q: scheme must be https", raw)
	}
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// llmOrigin normalises a base URL to scheme://host:port — the part that
// decides who receives the API key.
func llmOrigin(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return defaultLLMOrigin
	}
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}
