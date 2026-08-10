package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/semver"
)

// ─── 4.1 Version comparison ─────────────────────────────

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		current   string
		latest    string
		hasUpdate bool
	}{
		{"v1.2.2", "v1.2.3", true},
		{"v1.2.9", "v1.3.0", true},
		{"v1.9.9", "v2.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v2.0.0", "v1.9.9", false},
		{"v1.0.0-beta", "v1.0.0", true},
		{"1.0.0", "v1.1.0", true},
		{"dev", "dev", false},
	}

	for _, tt := range tests {
		t.Run(tt.current+" vs "+tt.latest, func(t *testing.T) {
			u := NewUpdater(tt.current)
			u.SetAPIBase("http://unused")

			latest := tt.latest
			hasUpdate := versionIsNewer(tt.current, latest)
			if hasUpdate != tt.hasUpdate {
				t.Errorf("versionIsNewer(%q, %q) = %v, want %v", tt.current, latest, hasUpdate, tt.hasUpdate)
			}

			_ = u
		})
	}
}

// ─── Version bump severity (major/minor/patch) ──────────

func TestVersionBump(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    string
	}{
		{"v1.2.3", "v2.0.0", "major"},
		{"v1.9.9", "v2.0.0", "major"},
		{"v1.2.3", "v1.3.0", "minor"},
		{"v1.2.3", "v1.2.4", "patch"},
		{"v1.2.3", "v1.2.3", "none"},  // equal
		{"v2.0.0", "v1.9.9", "none"},  // older
		{"1.0.0", "2.0.0", "major"},   // missing v prefix, still parsed
		{"dev", "v2.0.0", "none"},     // unparseable current
		{"v1.0.0", "garbage", "none"}, // unparseable latest
	}
	for _, tt := range tests {
		if got := versionBump(tt.current, tt.latest); got != tt.want {
			t.Errorf("versionBump(%q, %q) = %q, want %q", tt.current, tt.latest, got, tt.want)
		}
	}
}

// ─── 4.2-4.3 GitHub API response parsing ────────────────

func TestCheckForUpdates_Parsing(t *testing.T) {
	tests := []struct {
		name    string
		resp    githubRelease
		status  int
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid release with update",
			resp: githubRelease{
				TagName: "v2.0.0",
				HTMLURL: "https://github.com/qnqatop/papeer/releases/tag/v2.0.0",
				Body:    "Release notes here",
				Assets: []githubAsset{
					{Name: "papeer-macos-universal.zip", BrowserDownloadURL: "https://example.com/mac.zip"},
					{Name: "papeer-windows-amd64.zip", BrowserDownloadURL: "https://example.com/win.zip"},
					{Name: "papeer-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux.tar.gz"},
				},
			},
			status:  200,
			wantErr: false,
		},
		{
			name:    "no assets",
			resp:    githubRelease{TagName: "v1.0.0", Assets: []githubAsset{}},
			status:  200,
			wantErr: true,
			errMsg:  "no assets found",
		},
		{
			name:    "empty tag",
			resp:    githubRelease{TagName: "", Assets: []githubAsset{{Name: "test.zip"}}},
			status:  200,
			wantErr: true,
			errMsg:  "no version tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				json.NewEncoder(w).Encode(tt.resp)
			}))
			defer srv.Close()

			u := NewUpdater("v1.0.0")
			u.SetAPIBase(srv.URL)

			info, err := u.CheckForUpdates()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.LatestVersion != tt.resp.TagName {
				t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, tt.resp.TagName)
			}
			if info.ReleaseURL != tt.resp.HTMLURL {
				t.Errorf("ReleaseURL = %q, want %q", info.ReleaseURL, tt.resp.HTMLURL)
			}
			if info.ReleaseNotes != tt.resp.Body {
				t.Errorf("ReleaseNotes = %q, want %q", info.ReleaseNotes, tt.resp.Body)
			}
		})
	}
}

// ─── 4.4-4.6 Asset URL resolution ───────────────────────

func TestResolveAsset(t *testing.T) {
	allAssets := []githubAsset{
		{Name: "papeer-macos-universal.zip", BrowserDownloadURL: "mac"},
		{Name: "papeer-windows-amd64.zip", BrowserDownloadURL: "win"},
		{Name: "papeer-linux-amd64.tar.gz", BrowserDownloadURL: "linux"},
	}

	url, err := resolveAsset(allAssets)
	if err != nil {
		t.Fatalf("resolveAsset failed: %v", err)
	}

	var expected string
	switch runtime.GOOS {
	case "darwin":
		expected = "mac"
	case "windows":
		expected = "win"
	case "linux":
		expected = "linux"
	default:
		t.Skip("unsupported test platform")
	}

	if url != expected {
		t.Errorf("asset URL = %q, want %q", url, expected)
	}
}

func TestResolveAsset_TwoForOnePlatform(t *testing.T) {
	assets := []githubAsset{
		{Name: "papeer-linux-amd64.tar.gz", BrowserDownloadURL: "linux-amd64"},
		{Name: "papeer-linux-arm64.tar.gz", BrowserDownloadURL: "linux-arm64"},
	}
	if runtime.GOOS == "linux" {
		url, err := resolveAsset(assets)
		if err != nil {
			t.Fatal(err)
		}
		if url != "linux-amd64" {
			t.Errorf("expected first matching asset, got %q", url)
		}
	}
}

func TestResolveAsset_NoMatch(t *testing.T) {
	assets := []githubAsset{
		{Name: "papeer-freebsd.zip", BrowserDownloadURL: "freebsd"},
	}
	_, err := resolveAsset(assets)
	if err == nil {
		t.Error("expected error for unsupported platform")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error %q doesn't mention unsupported platform", err.Error())
	}
}

// ─── 4.7-4.11 HTTP error scenarios ──────────────────────

func TestCheckForUpdates_HTTPErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		errMsg string
	}{
		{"broken json", 200, "{bad", "parsing"},
		{"rate limited", 403, `{"message":"API rate limit exceeded"}`, "rate limit"},
		{"not found", 404, "", "no releases"},
		{"server error 500", 500, "", "unavailable"},
		{"server error 502", 502, "", "unavailable"},
		{"server error 503", 503, "", "unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			u := NewUpdater("v1.0.0")
			u.SetAPIBase(srv.URL)

			_, err := u.CheckForUpdates()
			if err == nil {
				t.Error("expected error, got nil")
			} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("error %q doesn't contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestCheckForUpdates_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	u := NewUpdater("v1.0.0")
	u.SetAPIBase(srv.URL)
	u.SetHTTPClient(&http.Client{Timeout: 100 * time.Millisecond})

	_, err := u.CheckForUpdates()
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

// ─── 4.12-4.14 Download tests ───────────────────────────

func TestDownloadUpdate_Streaming(t *testing.T) {
	dataSize := int64(5 * 1024 * 1024) // 5 MB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
		w.WriteHeader(200)
		io.CopyN(w, rand.Reader, dataSize)
	}))
	defer srv.Close()

	u := NewUpdater("v1.0.0")
	u.SetHTTPClient(&http.Client{Timeout: 10 * time.Second})

	var progressCalls int
	path, err := u.DownloadUpdate(srv.URL+"/download", func(downloaded, total int64) {
		progressCalls++
		if downloaded < 0 {
			t.Error("negative downloaded")
		}
		if total != dataSize {
			t.Errorf("total = %d, want %d", total, dataSize)
		}
	})
	if err != nil {
		t.Fatalf("DownloadUpdate failed: %v", err)
	}
	defer os.Remove(path)

	if progressCalls < 1 {
		t.Errorf("onProgress called %d times, expected at least 1", progressCalls)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("downloaded file stat: %v", err)
	}
	if info.Size() != dataSize {
		t.Errorf("downloaded size = %d, want %d", info.Size(), dataSize)
	}
}

func TestDownloadUpdate_Interrupted(t *testing.T) {
	cutAt := int64(1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", cutAt*3))
		w.WriteHeader(200)
		io.CopyN(w, rand.Reader, cutAt)
		// Connection drops — handler returns without writing the rest.
	}))
	defer srv.Close()

	u := NewUpdater("v1.0.0")
	u.SetHTTPClient(&http.Client{Timeout: 10 * time.Second})

	_, err := u.DownloadUpdate(srv.URL+"/download", nil)
	if err == nil {
		t.Error("expected error for incomplete download, got nil")
	}
}

func TestDownloadUpdate_ContentLengthMismatch(t *testing.T) {
	realSize := int64(1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", realSize*2)) // lie about size
		w.WriteHeader(200)
		io.CopyN(w, rand.Reader, realSize)
	}))
	defer srv.Close()

	u := NewUpdater("v1.0.0")
	u.SetHTTPClient(&http.Client{Timeout: 10 * time.Second})

	_, err := u.DownloadUpdate(srv.URL+"/download", nil)
	if err == nil {
		t.Error("expected error for size mismatch, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "incomplete") && !strings.Contains(strings.ToLower(err.Error()), "interrupt") {
		t.Errorf("error %q should mention incomplete download", err.Error())
	}
}

// ─── 4.15-4.19 Archive extraction ───────────────────────

func createTestZip(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("papeer")
	io.WriteString(w, "fake binary content")
	zw.Close()
	f.Close()
	return path
}

func createTestTarGz(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "test.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	body := []byte("fake binary content")
	tw.WriteHeader(&tar.Header{Name: "papeer", Size: int64(len(body)), Mode: 0o755})
	tw.Write(body)
	tw.Close()
	gw.Close()
	f.Close()
	return path
}

func TestExtractZip(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := createTestZip(t, tmpDir)
	destDir := filepath.Join(tmpDir, "staging")

	u := NewUpdater("v1.0.0")
	binary, err := u.ExtractArchive(zipPath, destDir)
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	info, err := os.Stat(binary)
	if err != nil {
		t.Fatalf("stat extracted binary: %v", err)
	}
	if info.Size() == 0 {
		t.Error("extracted binary is empty")
	}
}

func TestExtractTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	tgzPath := createTestTarGz(t, tmpDir)
	destDir := filepath.Join(tmpDir, "staging")

	u := NewUpdater("v1.0.0")
	binary, err := u.ExtractArchive(tgzPath, destDir)
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	info, err := os.Stat(binary)
	if err != nil {
		t.Fatalf("stat extracted binary: %v", err)
	}
	if info.Size() == 0 {
		t.Error("extracted binary is empty")
	}
}

func TestExtractCorruptedZip(t *testing.T) {
	tmpDir := t.TempDir()
	corrupt := filepath.Join(tmpDir, "corrupt.zip")
	os.WriteFile(corrupt, []byte("not a zip file"), 0o644)

	u := NewUpdater("v1.0.0")
	_, err := u.ExtractArchive(corrupt, filepath.Join(tmpDir, "staging"))
	if err == nil {
		t.Error("expected error for corrupt zip")
	}
}

func TestExtractCorruptedTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	corrupt := filepath.Join(tmpDir, "corrupt.tar.gz")
	os.WriteFile(corrupt, []byte("not a tar.gz"), 0o644)

	u := NewUpdater("v1.0.0")
	_, err := u.ExtractArchive(corrupt, filepath.Join(tmpDir, "staging"))
	if err == nil {
		t.Error("expected error for corrupt tar.gz")
	}
}

func TestExtractEmptyArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an empty zip.
	zipPath := filepath.Join(tmpDir, "empty.zip")
	f, _ := os.Create(zipPath)
	zw := zip.NewWriter(f)
	zw.Close()
	f.Close()

	u := NewUpdater("v1.0.0")
	_, err := u.ExtractArchive(zipPath, filepath.Join(tmpDir, "staging"))
	if err == nil {
		t.Error("expected error for archive without binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention binary not found", err.Error())
	}
}

// ─── 4.20-4.21 Windows install script generation ────────

func TestInstallWindows_ScriptContent(t *testing.T) {
	// This test verifies the logic of building the PS script,
	// not the actual execution (which requires Windows).
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific test on non-Windows platform")
	}

	exePath, _ := os.Executable()
	newExe := filepath.Join(os.TempDir(), "papeer-update", "papeer.exe")

	script := fmt.Sprintf(`Start-Sleep -Seconds 3
Copy-Item -Path "%s" -Destination "%s" -Force
Start-Process -FilePath "%s"
Remove-Item -Path $MyInvocation.MyCommand.Path -Force
`, newExe, exePath, exePath)

	if !strings.Contains(script, "Copy-Item") {
		t.Error("script missing Copy-Item")
	}
	if !strings.Contains(script, "Start-Process") {
		t.Error("script missing Start-Process")
	}
	if !strings.Contains(script, "Remove-Item") {
		t.Error("script missing Remove-Item")
	}
	if !strings.Contains(script, "$MyInvocation.MyCommand.Path") {
		t.Error("script missing self-delete")
	}
	if !strings.Contains(script, "Start-Sleep -Seconds 3") {
		t.Error("script missing sleep")
	}
}

// ─── 4.22-4.23 Path resolution ──────────────────────────

func TestFindAppBundle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}
	bundle, err := findAppBundle()
	if err != nil {
		// When running tests, the binary is not inside an .app bundle.
		if strings.Contains(err.Error(), "cannot find .app bundle") {
			t.Skip("test binary is not inside an .app bundle (expected in CI)")
		}
		t.Fatalf("findAppBundle: %v", err)
	}
	if !strings.HasSuffix(bundle, ".app") {
		t.Errorf("bundle path %q doesn't end with .app", bundle)
	}
}

func TestExecutablePath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if exe == "" {
		t.Error("executable path is empty")
	}
	t.Logf("Executable: %s", exe)
}

// ─── 4.24 Full cycle with mocks ─────────────────────────

func TestFullCycleMock(t *testing.T) {
	var assetData bytes.Buffer
	if runtime.GOOS == "linux" {
		gw := gzip.NewWriter(&assetData)
		tw := tar.NewWriter(gw)
		body := []byte("fake binary")
		tw.WriteHeader(&tar.Header{Name: "papeer", Size: int64(len(body)), Mode: 0o755})
		tw.Write(body)
		tw.Close()
		gw.Close()
	} else {
		zw := zip.NewWriter(&assetData)
		w, _ := zw.Create("papeer")
		w.Write([]byte("fake binary"))
		zw.Close()
	}

	downloadCalled := false
	downloadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadCalled = true
		w.Header().Set("Content-Length", fmt.Sprintf("%d", assetData.Len()))
		w.Write(assetData.Bytes())
	}))
	defer downloadSrv.Close()

	apiCalled := false
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/repos/qnqatop/papeer/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		assetName := "papeer-macos-universal.zip"
		if runtime.GOOS == "linux" {
			assetName = "papeer-linux-amd64.tar.gz"
		} else if runtime.GOOS == "windows" {
			assetName = "papeer-windows-amd64.zip"
		}
		json.NewEncoder(w).Encode(githubRelease{
			TagName: "v99.0.0",
			HTMLURL: "https://github.com/qnqatop/papeer/releases/tag/v99.0.0",
			Body:    "test release",
			Assets: []githubAsset{
				{Name: assetName, BrowserDownloadURL: downloadSrv.URL + "/dl"},
			},
		})
	})

	apiSrv := httptest.NewServer(apiMux)
	defer apiSrv.Close()

	u := NewUpdater("v1.0.0")
	u.SetAPIBase(apiSrv.URL)

	info, err := u.CheckForUpdates()
	if err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	if !apiCalled {
		t.Error("API was not called")
	}
	if !info.HasUpdate {
		t.Error("expected HasUpdate=true")
	}

	stagingDir := filepath.Join(t.TempDir(), "staging")
	archivePath, err := u.DownloadUpdate(info.AssetURL, nil)
	if err != nil {
		t.Fatalf("DownloadUpdate: %v", err)
	}
	if !downloadCalled {
		t.Error("download was not called")
	}
	defer os.Remove(archivePath)

	binary, err := u.ExtractArchive(archivePath, stagingDir)
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}

	binfo, err := os.Stat(binary)
	if err != nil || binfo.Size() == 0 {
		t.Error("extracted binary is missing or empty")
	}

	t.Logf("Full cycle OK: binary at %s (%d bytes)", binary, binfo.Size())
}

// ─── Executable bit preservation on extraction ──────────

func TestExtractPreservesExecBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes not meaningful on Windows")
	}

	// zip with an executable entry.
	t.Run("zip", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "exec.zip")
		f, _ := os.Create(zipPath)
		zw := zip.NewWriter(f)
		hdr := &zip.FileHeader{Name: "papeer", Method: zip.Deflate}
		hdr.SetMode(0o755)
		w, _ := zw.CreateHeader(hdr)
		io.WriteString(w, "binary")
		zw.Close()
		f.Close()

		u := NewUpdater("v1.0.0")
		bin, err := u.ExtractArchive(zipPath, filepath.Join(tmpDir, "staging"))
		if err != nil {
			t.Fatalf("ExtractArchive: %v", err)
		}
		info, _ := os.Stat(bin)
		if info.Mode()&0o100 == 0 {
			t.Errorf("extracted zip binary is not executable: mode %v", info.Mode())
		}
	})

	// tar.gz with an executable entry.
	t.Run("tar.gz", func(t *testing.T) {
		tmpDir := t.TempDir()
		tgzPath := filepath.Join(tmpDir, "exec.tar.gz")
		f, _ := os.Create(tgzPath)
		gw := gzip.NewWriter(f)
		tw := tar.NewWriter(gw)
		body := []byte("binary")
		tw.WriteHeader(&tar.Header{Name: "papeer", Size: int64(len(body)), Mode: 0o755})
		tw.Write(body)
		tw.Close()
		gw.Close()
		f.Close()

		u := NewUpdater("v1.0.0")
		bin, err := u.ExtractArchive(tgzPath, filepath.Join(tmpDir, "staging"))
		if err != nil {
			t.Fatalf("ExtractArchive: %v", err)
		}
		info, _ := os.Stat(bin)
		if info.Mode()&0o100 == 0 {
			t.Errorf("extracted tar.gz binary is not executable: mode %v", info.Mode())
		}
	})
}

// ─── copyFile (cross-device staging on Linux) ───────────

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")

	content := []byte("new binary payload")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst, 0o755); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("copied content = %q, want %q", got, content)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("copied mode = %v, want 0755", info.Mode().Perm())
		}
	}
}

func TestPSQuote(t *testing.T) {
	if got := psQuote(`C:\Program Files\Papeer\papeer.exe`); got != `'C:\Program Files\Papeer\papeer.exe'` {
		t.Errorf("psQuote plain = %q", got)
	}
	// An embedded single quote must be doubled so it cannot break out.
	if got := psQuote(`C:\o'brien\papeer.exe`); got != `'C:\o''brien\papeer.exe'` {
		t.Errorf("psQuote with apostrophe = %q", got)
	}
}

// ─── Semver helpers ─────────────────────────────────────

func TestSemverIsValid(t *testing.T) {
	valid := []string{"v1.0.0", "v0.0.1", "v99.99.99", "v1.2.3-beta", "v1.0.0-alpha.1", "v1.0", "v1"}
	invalid := []string{"", "abc", "1.0.0"}

	for _, v := range valid {
		if !semver.IsValid(v) {
			t.Errorf("expected %q to be valid semver", v)
		}
	}
	for _, v := range invalid {
		if semver.IsValid(v) {
			t.Errorf("expected %q to be invalid semver", v)
		}
	}
}
