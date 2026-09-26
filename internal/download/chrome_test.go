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

func TestChromeRequestAllowed(t *testing.T) {
	allowed := []string{"https://www.mdpi.com/x.pdf", "http://example.org/a"}
	for _, u := range allowed {
		if !chromeRequestAllowed(u) {
			t.Errorf("%s should be allowed", u)
		}
	}
	blocked := []string{
		"file:///etc/passwd", "chrome://settings", "javascript:alert(1)",
		"http://localhost:8080/", "http://127.0.0.1/", "http://[::1]/",
		"http://169.254.169.254/latest/meta-data/", "http://192.168.0.1/", "http://0.0.0.0/",
	}
	for _, u := range blocked {
		if chromeRequestAllowed(u) {
			t.Errorf("%s should be blocked", u)
		}
	}
}

func TestSetChromeProxy(t *testing.T) {
	t.Cleanup(func() { SetChromeProxy("") })
	SetChromeProxy("socks5://user:pass@127.0.0.1:1080")
	if got := currentChromeProxy(); got != "socks5://127.0.0.1:1080" {
		t.Errorf("proxy = %q", got)
	}
	SetChromeProxy("")
	if got := currentChromeProxy(); got != "" {
		t.Errorf("proxy after reset = %q", got)
	}
}
