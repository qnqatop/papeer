package llm

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/zalando/go-keyring"
)

// KeyringService is the OS keychain service name under which LLM API keys live.
const KeyringService = "papeer-llm"

// ErrKeyringUnavailable indicates the OS keychain could not be reached
// (headless Linux / CI). Callers should degrade gracefully: the profile is
// still usable, the key just has to be entered later.
var ErrKeyringUnavailable = errors.New("OS keychain is unavailable; the API key was not saved — enter it again later")

// account maps a profile id to a keyring account string.
func account(profileID int64) string { return strconv.FormatInt(profileID, 10) }

// SetKey stores the API key for a profile. Returns ErrKeyringUnavailable
// (wrapped) if the keychain backend cannot be reached — the caller decides
// whether that is fatal.
func SetKey(profileID int64, apiKey string) error {
	if err := keyring.Set(KeyringService, account(profileID), apiKey); err != nil {
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return nil
}

// GetKey returns the API key for a profile. A missing key returns ("", nil).
func GetKey(profileID int64) (string, error) {
	key, err := keyring.Get(KeyringService, account(profileID))
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return key, nil
}

// DeleteKey removes the API key for a profile. A missing key is not an error.
func DeleteKey(profileID int64) error {
	err := keyring.Delete(KeyringService, account(profileID))
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return nil
}

// MaskKey returns a display-safe mask of an API key: first 3 + last 4 chars,
// e.g. "sk-…a1b2". Short or empty keys return "" (nothing to show).
func MaskKey(apiKey string) string {
	if len(apiKey) < 8 {
		if apiKey == "" {
			return ""
		}
		return "…"
	}
	return apiKey[:3] + "…" + apiKey[len(apiKey)-4:]
}
