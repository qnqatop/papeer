package llm

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringRoundTrip(t *testing.T) {
	keyring.MockInit()

	if err := SetKey(1, "sk-secret-123"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := GetKey(1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "sk-secret-123" {
		t.Errorf("got %q", got)
	}

	// Missing key returns empty, not error.
	got, err = GetKey(2)
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty for missing key, got %q", got)
	}

	if err := DeleteKey(1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ = GetKey(1)
	if got != "" {
		t.Errorf("expected empty after delete, got %q", got)
	}

	// Deleting a missing key is not an error.
	if err := DeleteKey(1); err != nil {
		t.Errorf("delete missing: %v", err)
	}
}

func TestKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)

	err := SetKey(5, "x")
	if err == nil {
		t.Fatal("expected error when keychain unavailable")
	}
	// Restore a working mock for other tests.
	keyring.MockInit()
}

func TestMaskKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"short", "…"},
		{"sk-1234567890a1b2", "sk-…a1b2"},
		{"AIzaSyABCDEFGH", "AIz…EFGH"},
	}
	for _, tt := range tests {
		if got := MaskKey(tt.in); got != tt.want {
			t.Errorf("MaskKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
