package download

import "testing"

func TestFindChromeBinary_Honored(t *testing.T) {
	// Smoke test: function must not panic and must return either "" or a
	// path to a file that exists. We do not assert which case applies —
	// CI runners may or may not have Chrome installed.
	got := findChromeBinary()
	if got == "" {
		t.Skip("no chrome binary on host — skipping")
	}
	if !ChromeAvailable() {
		t.Errorf("ChromeAvailable() = false but findChromeBinary() = %q", got)
	}
}
