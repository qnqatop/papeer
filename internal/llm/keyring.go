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

// secretAccount maps a named app secret (e.g. "semantic_scholar_api_key") to
// a keyring account string. The "secret:" prefix keeps it apart from the
// numeric LLM profile accounts.
func secretAccount(name string) string { return "secret:" + name }

// SetKey stores the API key for a profile. Returns ErrKeyringUnavailable
// (wrapped) if the keychain backend cannot be reached — the caller decides
// whether that is fatal.
func SetKey(profileID int64, apiKey string) error { return setEntry(account(profileID), apiKey) }

// GetKey returns the API key for a profile. A missing key returns ("", nil).
func GetKey(profileID int64) (string, error) { return getEntry(account(profileID)) }

// DeleteKey removes the API key for a profile. A missing key is not an error.
func DeleteKey(profileID int64) error { return deleteEntry(account(profileID)) }

// SetSecret stores a named app secret (not tied to an LLM profile) in the
// keychain. Errors wrap ErrKeyringUnavailable like SetKey.
func SetSecret(name, value string) error { return setEntry(secretAccount(name), value) }

// GetSecret returns a named app secret. A missing secret returns ("", nil).
func GetSecret(name string) (string, error) { return getEntry(secretAccount(name)) }

// DeleteSecret removes a named app secret. A missing secret is not an error.
func DeleteSecret(name string) error { return deleteEntry(secretAccount(name)) }

func setEntry(acct, value string) error {
	if err := keyring.Set(KeyringService, acct, value); err != nil {
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return nil
}

func getEntry(acct string) (string, error) {
	v, err := keyring.Get(KeyringService, acct)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	return v, nil
}

func deleteEntry(acct string) error {
	err := keyring.Delete(KeyringService, acct)
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
