package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// ─── Helpers: signed mock release ───────────────────────

func testKeys(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub), priv
}

// withPublicKey sets the embedded PublicKey for the duration of the test.
func withPublicKey(t *testing.T, pub string) {
	t.Helper()
	old := PublicKey
	PublicKey = pub
	t.Cleanup(func() { PublicKey = old })
}

func platformAssetName() string {
	switch runtime.GOOS {
	case "linux":
		return "papeer-linux-amd64.tar.gz"
	case "windows":
		return "papeer-windows-amd64.zip"
	default:
		return "papeer-macos-universal.zip"
	}
}

// releaseServer serves a GitHub-like "latest release" plus its assets.
type releaseServer struct {
	srv *httptest.Server

	mu          sync.Mutex
	archive     []byte
	sums        []byte // nil: release has no SHA256SUMS
	sig         []byte // nil: release has no SHA256SUMS.sig
	size        int64  // advertised archive size
	archiveHits int
}

func newReleaseServer(t *testing.T, archive []byte) *releaseServer {
	t.Helper()
	rs := &releaseServer{archive: archive, size: int64(len(archive))}
	rs.sums = []byte(fmt.Sprintf("%x  %s\n%x  other-asset.zip\n",
		sha256.Sum256(archive), platformAssetName(), sha256.Sum256([]byte("x"))))

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/qnqatop/papeer/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		base := rs.srv.URL
		assets := []githubAsset{{Name: platformAssetName(), BrowserDownloadURL: base + "/dl/archive", Size: rs.size}}
		if rs.sums != nil {
			assets = append(assets, githubAsset{Name: checksumsAsset, BrowserDownloadURL: base + "/dl/sums"})
		}
		if rs.sig != nil {
			assets = append(assets, githubAsset{Name: signatureAsset, BrowserDownloadURL: base + "/dl/sig"})
		}
		json.NewEncoder(w).Encode(githubRelease{TagName: "v99.0.0", Assets: assets})
	})
	serve := func(get func() []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			rs.mu.Lock()
			data := get()
			rs.mu.Unlock()
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.Write(data)
		}
	}
	mux.HandleFunc("/dl/archive", serve(func() []byte { rs.archiveHits++; return rs.archive }))
	mux.HandleFunc("/dl/sums", serve(func() []byte { return rs.sums }))
	mux.HandleFunc("/dl/sig", serve(func() []byte { return rs.sig }))

	rs.srv = httptest.NewServer(mux)
	t.Cleanup(rs.srv.Close)
	return rs
}

func (rs *releaseServer) set(f func(rs *releaseServer)) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	f(rs)
}

func (rs *releaseServer) signWith(priv ed25519.PrivateKey) {
	rs.set(func(rs *releaseServer) {
		rs.sig = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, rs.sums)) + "\n")
	})
}

func (rs *releaseServer) hits() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.archiveHits
}

// updater returns an Updater pointed at the mock that has already run
// CheckForUpdates.
func (rs *releaseServer) updater(t *testing.T) *Updater {
	t.Helper()
	u := NewUpdater("v1.0.0")
	u.SetAPIBase(rs.srv.URL)
	u.trustedOrigin = rs.srv.URL
	return u
}

func checkedUpdater(t *testing.T, rs *releaseServer) *Updater {
	t.Helper()
	u := rs.updater(t)
	if _, err := u.CheckForUpdates(); err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	return u
}

// isolateTemp points os.TempDir at a fresh directory so tests can assert
// that rejected downloads leave nothing behind.
func isolateTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	return dir
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Errorf("leftover file after rejected download: %s", e.Name())
	}
}

var payload = []byte("archive bytes")

// ─── Signature & checksum verification ──────────────────

func TestDownloadUpdate_SignatureOK(t *testing.T) {
	pub, priv := testKeys(t)
	withPublicKey(t, pub)
	rs := newReleaseServer(t, payload)
	rs.signWith(priv)

	path, err := checkedUpdater(t, rs).DownloadUpdate(context.Background(), nil)
	if err != nil {
		t.Fatalf("DownloadUpdate: %v", err)
	}
	defer os.Remove(path)
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, payload) {
		t.Errorf("downloaded %q, want %q", got, payload)
	}
}

func TestDownloadUpdate_BadSignature(t *testing.T) {
	pub, _ := testKeys(t)
	_, otherPriv := testKeys(t)
	withPublicKey(t, pub)
	rs := newReleaseServer(t, payload)
	rs.signWith(otherPriv) // signed with the wrong key

	_, err := checkedUpdater(t, rs).DownloadUpdate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature error, got %v", err)
	}
	if rs.hits() != 0 {
		t.Error("archive must not be downloaded when the signature is bad")
	}
}

func TestDownloadUpdate_TamperedChecksums(t *testing.T) {
	pub, priv := testKeys(t)
	withPublicKey(t, pub)
	rs := newReleaseServer(t, payload)
	rs.signWith(priv)
	rs.set(func(rs *releaseServer) { rs.sums = append([]byte{}, rs.sums...); rs.sums[0] ^= 1 })

	_, err := checkedUpdater(t, rs).DownloadUpdate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature error, got %v", err)
	}
}

func TestDownloadUpdate_MissingSignature(t *testing.T) {
	pub, _ := testKeys(t)
	withPublicKey(t, pub)
	rs := newReleaseServer(t, payload) // no .sig

	_, err := checkedUpdater(t, rs).DownloadUpdate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), signatureAsset) {
		t.Fatalf("expected missing-signature error, got %v", err)
	}
}

func TestDownloadUpdate_NoPublicKeyHashOnly(t *testing.T) {
	withPublicKey(t, "")
	rs := newReleaseServer(t, payload) // no .sig, integrity only

	path, err := checkedUpdater(t, rs).DownloadUpdate(context.Background(), nil)
	if err != nil {
		t.Fatalf("DownloadUpdate without key: %v", err)
	}
	os.Remove(path)
}

func TestDownloadUpdate_MissingChecksums(t *testing.T) {
	withPublicKey(t, "")
	rs := newReleaseServer(t, payload)
	rs.set(func(rs *releaseServer) { rs.sums = nil })

	_, err := checkedUpdater(t, rs).DownloadUpdate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), checksumsAsset) {
		t.Fatalf("expected missing SHA256SUMS error, got %v", err)
	}
	if rs.hits() != 0 {
		t.Error("archive must not be downloaded without SHA256SUMS")
	}
}

func TestDownloadUpdate_HashMismatch(t *testing.T) {
	pub, priv := testKeys(t)
	withPublicKey(t, pub)
	rs := newReleaseServer(t, payload)
	rs.set(func(rs *releaseServer) { rs.archive = []byte("evil bytes!!!") }) // same length, different content
	rs.signWith(priv)

	u := checkedUpdater(t, rs)
	tmp := isolateTemp(t)
	_, err := u.DownloadUpdate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	assertEmptyDir(t, tmp)
}

func TestDownloadUpdate_NoEntryForAsset(t *testing.T) {
	withPublicKey(t, "")
	rs := newReleaseServer(t, payload)
	rs.set(func(rs *releaseServer) {
		rs.sums = []byte(fmt.Sprintf("%x  something-else.zip\n", sha256.Sum256(payload)))
	})

	_, err := checkedUpdater(t, rs).DownloadUpdate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Fatalf("expected missing entry error, got %v", err)
	}
}

func TestDownloadUpdate_RequiresCheck(t *testing.T) {
	u := NewUpdater("v1.0.0")
	if _, err := u.DownloadUpdate(context.Background(), nil); err == nil {
		t.Fatal("expected error without a prior CheckForUpdates")
	}

	// A check that finds no newer version leaves nothing to download.
	rs := newReleaseServer(t, payload)
	u = rs.updater(t)
	u.currentVersion = "v100.0.0"
	if _, err := u.CheckForUpdates(); err != nil {
		t.Fatal(err)
	}
	if _, err := u.DownloadUpdate(context.Background(), nil); err == nil {
		t.Fatal("expected error when no update is available")
	}
}

func TestParseChecksums(t *testing.T) {
	a := sha256.Sum256([]byte("a"))
	data := fmt.Sprintf("%x  a.zip\n%x *b.tar.gz\n\n", a, a)
	sums, err := parseChecksums([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sums["a.zip"], a[:]) || !bytes.Equal(sums["b.tar.gz"], a[:]) {
		t.Errorf("unexpected parse result: %v", sums)
	}

	for _, bad := range []string{
		"nothex  a.zip\n",
		"abcd  a.zip\n",
		fmt.Sprintf("%x\n", a),
		fmt.Sprintf("%x  a.zip\n%x  a.zip\n", a, sha256.Sum256([]byte("b"))),
	} {
		if _, err := parseChecksums([]byte(bad)); err == nil {
			t.Errorf("parseChecksums(%q) should fail", bad)
		}
	}
}

func TestVerifySignature(t *testing.T) {
	pub, priv := testKeys(t)
	msg := []byte("checksums")
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg)))

	if err := VerifySignature(pub, msg, sig); err != nil {
		t.Errorf("valid signature rejected: %v", err)
	}
	if err := VerifySignature(pub, []byte("other"), sig); err == nil {
		t.Error("signature over different message accepted")
	}
	if err := VerifySignature("not base64!", msg, sig); err == nil {
		t.Error("malformed public key accepted")
	}
	if err := VerifySignature(pub, msg, []byte("AAAA")); err == nil {
		t.Error("short signature accepted")
	}
}

// ─── Download URL allowlist & size caps ─────────────────

func TestCheckAssetURL(t *testing.T) {
	u := NewUpdater("v1.0.0")
	allowed := []string{
		"https://github.com/qnqatop/papeer/releases/download/v1/a.zip",
		"https://objects.githubusercontent.com/x",
		"https://release-assets.githubusercontent.com/x",
	}
	rejected := []string{
		"http://github.com/qnqatop/papeer/releases/download/v1/a.zip",
		"https://evil.example.com/a.zip",
		"https://github.com.evil.example/a.zip",
		"file:///etc/passwd",
		"http://127.0.0.1:19999/dl",
	}
	for _, raw := range allowed {
		if err := u.checkAssetURL(raw); err != nil {
			t.Errorf("checkAssetURL(%q) = %v, want nil", raw, err)
		}
	}
	for _, raw := range rejected {
		if err := u.checkAssetURL(raw); err == nil {
			t.Errorf("checkAssetURL(%q) = nil, want error", raw)
		}
	}
}

func TestDownload_RejectsUntrustedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("untrusted server must not be contacted")
	}))
	defer srv.Close()

	u := NewUpdater("v1.0.0") // srv is not the trusted origin
	if _, _, err := u.downloadAsset(context.Background(), githubAsset{BrowserDownloadURL: srv.URL + "/a"}, nil); err == nil {
		t.Fatal("expected untrusted URL error")
	}
}

func TestDownload_RedirectAllowlist(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect target outside the allowlist must not be contacted")
	}))
	defer evil.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/evil":
			http.Redirect(w, r, evil.URL+"/payload", http.StatusFound)
		case "/local":
			http.Redirect(w, r, "/payload", http.StatusFound)
		default:
			w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()

	u := NewUpdater("v1.0.0")
	u.trustedOrigin = srv.URL

	if _, _, err := u.downloadAsset(context.Background(), githubAsset{BrowserDownloadURL: srv.URL + "/evil"}, nil); err == nil {
		t.Error("redirect to an untrusted host was followed")
	}
	path, _, err := u.downloadAsset(context.Background(), githubAsset{BrowserDownloadURL: srv.URL + "/local"}, nil)
	if err != nil {
		t.Fatalf("redirect within the allowlist failed: %v", err)
	}
	os.Remove(path)
}

func TestDownload_SizeCaps(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chunked" {
			// No Content-Length: the cap must be enforced while streaming.
			w.(http.Flusher).Flush()
		} else {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		}
		w.Write(body)
	}))
	defer srv.Close()

	u := NewUpdater("v1.0.0")
	u.trustedOrigin = srv.URL
	tmp := isolateTemp(t)

	// Content-Length above the advertised asset size: rejected up front.
	_, _, err := u.downloadAsset(context.Background(), githubAsset{BrowserDownloadURL: srv.URL + "/cl", Size: 1000}, nil)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected too-large error, got %v", err)
	}

	// Streamed body exceeding the advertised size: aborted mid-way.
	_, _, err = u.downloadAsset(context.Background(), githubAsset{BrowserDownloadURL: srv.URL + "/chunked", Size: 1000}, nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected exceeds error, got %v", err)
	}
	assertEmptyDir(t, tmp)

	// Unknown size (0) falls back to the hard ceiling and succeeds.
	path, _, err := u.downloadAsset(context.Background(), githubAsset{BrowserDownloadURL: srv.URL + "/cl"}, nil)
	if err != nil {
		t.Fatalf("download with unknown size: %v", err)
	}
	os.Remove(path)
}

func TestDownload_Cancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	u := NewUpdater("v1.0.0")
	u.trustedOrigin = srv.URL
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := u.downloadAsset(ctx, githubAsset{BrowserDownloadURL: srv.URL}, nil); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ─── Concurrency guard ──────────────────────────────────

func TestPrepareUpdate_RejectsConcurrentCalls(t *testing.T) {
	u := NewUpdater("v1.0.0")
	if err := u.begin(); err != nil {
		t.Fatal(err)
	}
	if err := u.PrepareUpdate(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("PrepareUpdate while busy: %v", err)
	}
	if err := u.InstallStaged(); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("InstallStaged while busy: %v", err)
	}
	u.end()
	if err := u.InstallStaged(); err == nil || !strings.Contains(err.Error(), "no downloaded update") {
		t.Errorf("InstallStaged without staging: %v", err)
	}
}

// ─── Extraction hardening ───────────────────────────────

type zipEntry struct {
	name, body string
	symlink    bool
}

func writeZip(t *testing.T, path string, entries []zipEntry) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.symlink {
			hdr.SetMode(os.ModeSymlink | 0o777)
		} else {
			hdr.SetMode(0o755)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body))
	}
	zw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtractZip_SymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	cases := map[string][]zipEntry{
		"absolute target": {
			{name: "link", body: "/etc", symlink: true},
			{name: "link/pwned", body: "x"},
		},
		"relative escape": {
			{name: "a/link", body: "../../outside", symlink: true},
			{name: "a/link/pwned", body: "x"},
		},
		"dot-dot target": {
			{name: "link", body: "..", symlink: true},
		},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			zipPath := filepath.Join(tmp, "evil.zip")
			writeZip(t, zipPath, append(entries, zipEntry{name: "papeer", body: "bin"}))

			_, err := NewUpdater("v1.0.0").ExtractArchive(zipPath, filepath.Join(tmp, "staging"))
			if err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("expected symlink rejection, got %v", err)
			}
			if _, err := os.Lstat(filepath.Join(tmp, "outside")); err == nil {
				t.Error("file written outside the staging dir")
			}
		})
	}
}

func TestExtractZip_NoWriteThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	// A symlink that already points outside destDir (planted, or created by
	// some other path) must never be written through.
	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside")
	os.MkdirAll(outside, 0o755)
	dest := filepath.Join(tmp, "staging")
	os.MkdirAll(dest, 0o755)
	if err := os.Symlink(outside, filepath.Join(dest, "dir")); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(tmp, "a.zip")
	writeZip(t, zipPath, []zipEntry{{name: "dir/pwned", body: "x"}, {name: "papeer", body: "bin"}})
	if _, err := NewUpdater("v1.0.0").ExtractArchive(zipPath, dest); err == nil {
		t.Error("expected error writing through an escaping symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned")); err == nil {
		t.Error("file written outside the staging dir through a symlink")
	}
}

func TestExtractZip_InternalSymlinkAllowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	// macOS frameworks: Versions/Current -> A.
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "fw.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "Fw/Versions/A/lib", body: "lib"},
		{name: "Fw/Versions/Current", body: "A", symlink: true},
		{name: "papeer", body: "bin"},
	})
	dest := filepath.Join(tmp, "staging")
	if _, err := NewUpdater("v1.0.0").ExtractArchive(zipPath, dest); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "Fw/Versions/Current/lib")); string(got) != "lib" {
		t.Errorf("internal symlink not usable, read %q", got)
	}
}

func TestExtractZip_PathTraversal(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "slip.zip")
	writeZip(t, zipPath, []zipEntry{{name: "../escape", body: "x"}})
	_, err := NewUpdater("v1.0.0").ExtractArchive(zipPath, filepath.Join(tmp, "staging"))
	if err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("expected invalid path error, got %v", err)
	}
}

func writeTarGz(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0o755, Typeflag: tar.TypeReg})
		tw.Write(body)
	}
	tw.Close()
	gw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtract_Limits(t *testing.T) {
	oldBytes, oldEntries := maxExtractBytes, maxExtractEntries
	maxExtractBytes, maxExtractEntries = 1000, 3
	t.Cleanup(func() { maxExtractBytes, maxExtractEntries = oldBytes, oldEntries })

	big := strings.Repeat("x", 600)
	tests := []struct {
		name    string
		zip     []zipEntry
		tar     map[string][]byte
		wantErr string
	}{
		{
			name:    "too many entries",
			zip:     []zipEntry{{name: "a"}, {name: "b"}, {name: "c"}, {name: "papeer", body: "bin"}},
			tar:     map[string][]byte{"a": nil, "b": nil, "c": nil, "papeer": []byte("bin")},
			wantErr: "too many entries",
		},
		{
			name:    "too many bytes",
			zip:     []zipEntry{{name: "a", body: big}, {name: "papeer", body: big}},
			tar:     map[string][]byte{"a": []byte(big), "papeer": []byte(big)},
			wantErr: "expands beyond",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/zip", func(t *testing.T) {
			tmp := t.TempDir()
			p := filepath.Join(tmp, "a.zip")
			writeZip(t, p, tt.zip)
			_, err := NewUpdater("v1.0.0").ExtractArchive(p, filepath.Join(tmp, "staging"))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got %v, want %q", err, tt.wantErr)
			}
		})
		t.Run(tt.name+"/tar.gz", func(t *testing.T) {
			tmp := t.TempDir()
			p := filepath.Join(tmp, "a.tar.gz")
			writeTarGz(t, p, tt.tar)
			_, err := NewUpdater("v1.0.0").ExtractArchive(p, filepath.Join(tmp, "staging"))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// ─── Linux binary replacement ───────────────────────────

func TestReplaceLinuxBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rename over a file differs on Windows")
	}
	tmp := t.TempDir()
	staging := filepath.Join(tmp, "staging")
	os.MkdirAll(staging, 0o755)
	// A decoy that sorts before "papeer" must not be picked.
	os.WriteFile(filepath.Join(staging, "LICENSE"), []byte("license"), 0o644)
	os.WriteFile(filepath.Join(staging, "papeer"), []byte("new"), 0o755)

	exe := filepath.Join(tmp, "bin", "papeer")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("old"), 0o755)
	os.WriteFile(exe+".new", []byte("stale"), 0o644) // leftover from a failed run

	if err := replaceLinuxBinary(staging, exe); err != nil {
		t.Fatalf("replaceLinuxBinary: %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Errorf("installed binary = %q, want %q", got, "new")
	}
	if got, _ := os.ReadFile(exe + ".bak"); string(got) != "old" {
		t.Errorf("backup = %q, want %q", got, "old")
	}
	if _, err := os.Lstat(exe + ".new"); err == nil {
		t.Error(".new left behind")
	}

	// Without a file named "papeer" the install is refused.
	os.Remove(filepath.Join(staging, "papeer"))
	if err := replaceLinuxBinary(staging, exe); err == nil {
		t.Error("expected error when the expected binary is missing")
	}
}
