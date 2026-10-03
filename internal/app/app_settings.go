package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/qnqatop/papeer/internal/httpclient"
	"github.com/qnqatop/papeer/internal/llm"
)

// ── Settings ────────────────────────────────────────────

// s2KeySetting is the legacy app_settings key of the Semantic Scholar API key
// and the name of its keychain secret.
const s2KeySetting = "semantic_scholar_api_key"

// allowedSettingKeys are the app_settings keys the webview may write through
// SaveSetting. Secrets have dedicated RPCs and are never accepted here.
var allowedSettingKeys = map[string]bool{
	"proxy_url":                true,
	"enable_radar":             true,
	"radar_frequency":          true,
	"llm_summary_prompt":       true,
	"auto_update_check":        true,
	"update_dismissed_version": true,
}

// secretSettingKeys are app_settings keys that may hold secrets (legacy
// plaintext keys awaiting migration). GetSettings never returns them.
var secretSettingKeys = []string{
	s2KeySetting,
	"llm_deepseek_api_key",
	"llm_gemini_api_key",
}

// GetSettings returns the non-secret app settings.
func (a *App) GetSettings() (map[string]string, error) {
	settings, err := a.db.GetAllSettings()
	if err != nil {
		return nil, err
	}
	for _, k := range secretSettingKeys {
		delete(settings, k)
	}
	return settings, nil
}

// SaveSetting stores one whitelisted setting. Changing the radar settings
// reschedules the radar without an app restart.
func (a *App) SaveSetting(key, value string) error {
	if !allowedSettingKeys[key] {
		return fmt.Errorf("unknown setting %q", key)
	}
	if err := a.db.SetSetting(key, value); err != nil {
		return err
	}
	if key == "enable_radar" || key == "radar_frequency" {
		a.restartRadar()
	}
	return nil
}

// TestProxy checks that the proxy URL works by fetching a tiny response from
// OpenAlex through it, with the same SSRF guard the real clients use.
func (a *App) TestProxy(proxyURL string) error {
	client, err := httpclient.NewWithProxy("", proxyURL)
	if err != nil {
		return err
	}
	client.EnableSSRFGuard()

	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	var result map[string]interface{}
	return client.DoJSON(ctx, "https://api.openalex.org/works?per-page=1", &result)
}

// ── Semantic Scholar API key ────────────────────────────

// S2KeyStatus tells the frontend whether a Semantic Scholar key is set,
// with a masked form for display. The raw key never leaves the backend.
type S2KeyStatus struct {
	Set    bool   `json:"set"`
	Masked string `json:"masked"`
}

// SetSemanticScholarKey stores the Semantic Scholar API key in the OS
// keychain; an empty key removes it. Any legacy plaintext copy in the DB is
// wiped once the keychain holds the new value.
func (a *App) SetSemanticScholarKey(key string) error {
	key = strings.TrimSpace(key)

	var err error
	if key == "" {
		err = llm.DeleteSecret(s2KeySetting)
	} else {
		err = llm.SetSecret(s2KeySetting, key)
	}
	if err != nil {
		return err
	}
	if err := a.db.DeleteSetting(s2KeySetting); err != nil {
		a.logWarningf("s2 key: failed to remove plaintext copy: %v", err)
	}

	a.s2KeyMu.Lock()
	a.s2Key = key
	a.s2KeyLoaded = true
	a.s2KeyMu.Unlock()
	return nil
}

// GetSemanticScholarKeyStatus reports whether a Semantic Scholar key is set.
func (a *App) GetSemanticScholarKeyStatus() (S2KeyStatus, error) {
	key := a.semanticScholarKey()
	return S2KeyStatus{Set: key != "", Masked: llm.MaskKey(key)}, nil
}

// semanticScholarKey returns the cached Semantic Scholar key, loading it on
// first use from the keychain or, if the keychain is unavailable, from the
// legacy plaintext setting.
func (a *App) semanticScholarKey() string {
	a.s2KeyMu.Lock()
	defer a.s2KeyMu.Unlock()
	if !a.s2KeyLoaded {
		a.s2Key = a.loadS2Key()
		a.s2KeyLoaded = true
	}
	return a.s2Key
}

// loadS2Key reads the key from the keychain, falling back to the plaintext
// DB copy kept when migration could not reach the keychain.
func (a *App) loadS2Key() string {
	key, err := llm.GetSecret(s2KeySetting)
	if err == nil && key != "" {
		return key
	}
	if err != nil {
		a.logWarningf("s2 key: keychain unavailable, using plaintext fallback: %v", err)
	}
	if a.db == nil {
		return ""
	}
	v, _ := a.db.GetSetting(s2KeySetting)
	return v
}

// migrateS2Key performs a one-time migration of the plaintext Semantic
// Scholar key from app_settings into the OS keychain, then deletes the DB
// row. If the keychain is unavailable the plaintext key is kept (so it is not
// lost), used as a fallback, and the migration is retried on a later launch.
func (a *App) migrateS2Key() {
	v, _ := a.db.GetSetting(s2KeySetting)
	if v == "" {
		return // nothing to migrate
	}
	if err := llm.SetSecret(s2KeySetting, v); err != nil {
		a.logWarningf("s2 key migration: keychain unavailable, keeping plaintext key for retry: %v", err)
		return
	}
	if err := a.db.DeleteSetting(s2KeySetting); err != nil {
		a.logWarningf("s2 key migration: failed to remove plaintext copy: %v", err)
	}
	a.s2KeyMu.Lock()
	a.s2Key = v
	a.s2KeyLoaded = true
	a.s2KeyMu.Unlock()
}

// ── Logging ─────────────────────────────────────────────

// hasWailsLogger reports whether ctx carries the Wails runtime logger; the
// runtime log functions exit the process without it (e.g. in tests).
func hasWailsLogger(ctx context.Context) bool {
	return ctx != nil && ctx.Value("logger") != nil
}

// logWarningf logs through the Wails runtime, or the standard logger when
// there is no Wails context.
func (a *App) logWarningf(format string, args ...interface{}) {
	if hasWailsLogger(a.ctx) {
		runtime.LogWarningf(a.ctx, format, args...)
		return
	}
	log.Printf("WARN: "+format, args...)
}

// logErrorf is logWarningf at error level.
func (a *App) logErrorf(format string, args ...interface{}) {
	if hasWailsLogger(a.ctx) {
		runtime.LogErrorf(a.ctx, format, args...)
		return
	}
	log.Printf("ERROR: "+format, args...)
}
