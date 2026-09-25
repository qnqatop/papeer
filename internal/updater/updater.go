package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/mod/semver"
)

const (
	repoOwner = "qnqatop"
	repoName  = "papeer"
	githubAPI = "https://api.github.com"
	userAgent = "papeer-updater"

	// maxDownloadBytes is the hard ceiling for a release archive, regardless
	// of the size the API advertises.
	maxDownloadBytes = 512 << 20
	// maxChecksumsBytes bounds SHA256SUMS / SHA256SUMS.sig downloads.
	maxChecksumsBytes = 64 << 10
	// maxSymlinkTarget bounds the size of a zip symlink entry's content.
	maxSymlinkTarget = 4096

	// linuxBinaryName is the executable name inside the Linux release tarball.
	linuxBinaryName = "papeer"
)

// maxExtractBytes and maxExtractEntries bound archive extraction
// (decompression bombs). Variables only so tests can lower them.
var (
	maxExtractBytes   int64 = 1 << 30
	maxExtractEntries       = 10000
)

// allowedAssetHosts are the only hosts release assets may be downloaded from
// (including every redirect hop). The scheme must be https.
var allowedAssetHosts = map[string]bool{
	"github.com":                           true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

type UpdateInfo struct {
	HasUpdate      bool   `json:"hasUpdate"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	AssetURL       string `json:"assetURL"`
	ReleaseURL     string `json:"releaseURL"`
	ReleaseNotes   string `json:"releaseNotes"`
	// Severity is the semver bump of LatestVersion over CurrentVersion:
	// "major", "minor", "patch", or "none" (no update / unparseable).
	Severity string `json:"severity"`
}

// pendingRelease holds the assets found by the last CheckForUpdates that
// reported an update. Downloads only ever use these server-provided assets,
// never a caller-supplied URL.
type pendingRelease struct {
	archive   githubAsset
	checksums *githubAsset // SHA256SUMS; nil when the release lacks it
	signature *githubAsset // SHA256SUMS.sig; nil when the release lacks it
}

type Updater struct {
	currentVersion string
	apiBase        string
	// trustedOrigin is an extra scheme://host assets may be fetched from in
	// addition to allowedAssetHosts (mock server in `mockupdate` builds,
	// httptest servers in tests). Empty in release builds.
	trustedOrigin string
	httpClient    *http.Client // GitHub API calls (short total timeout)
	dlClient      *http.Client // asset downloads (no total timeout)

	mu         sync.Mutex // guards the fields below
	pending    *pendingRelease
	stagingDir string
	busy       bool // a download or install is in progress
}

func NewUpdater(version string) *Updater {
	apiBase, trustedOrigin := defaultAPIBase()
	u := &Updater{
		currentVersion: version,
		apiBase:        apiBase,
		trustedOrigin:  trustedOrigin,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
	// Asset downloads can legitimately take minutes on a slow link, so there
	// is no total Timeout: only the wait for response headers is bounded, and
	// the request context allows cancellation.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 30 * time.Second
	u.dlClient = &http.Client{Transport: tr, CheckRedirect: u.checkRedirect}
	return u
}

func (u *Updater) SetAPIBase(base string) {
	u.apiBase = base
}

func (u *Updater) SetHTTPClient(c *http.Client) {
	u.httpClient = c
}

// checkAssetURL enforces the download allowlist: https on a GitHub release
// host, or the trusted origin (mock/test builds only).
func (u *Updater) checkAssetURL(raw string) error {
	p, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
	}
	if u.trustedOrigin != "" && p.Scheme+"://"+p.Host == u.trustedOrigin {
		return nil
	}
	if p.Scheme != "https" || !allowedAssetHosts[strings.ToLower(p.Hostname())] {
		return fmt.Errorf("refusing to download from untrusted URL %q", raw)
	}
	return nil
}

// checkRedirect applies the download allowlist to every redirect hop.
func (u *Updater) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return u.checkAssetURL(req.URL.String())
}

func platformKey() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	case "linux":
		return "linux"
	default:
		return runtime.GOOS
	}
}

func archiveExt() string {
	if runtime.GOOS == "linux" {
		return ".tar.gz"
	}
	return ".zip"
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func normalizeVersion(v string) string {
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// versionIsNewer reports whether latest is a strictly newer semver than
// current. Unparseable versions (e.g. "dev" builds, odd tags) never count as
// an update, so a dev build is not "updated" to an arbitrary or older tag.
func versionIsNewer(current, latest string) bool {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)
	if !semver.IsValid(current) || !semver.IsValid(latest) {
		return false
	}
	return semver.Compare(latest, current) > 0
}

// versionBump reports the semver significance of latest over current:
// "major", "minor", "patch", or "none" when latest is not newer (or either
// version is unparseable).
func versionBump(current, latest string) string {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)
	if !semver.IsValid(current) || !semver.IsValid(latest) || semver.Compare(latest, current) <= 0 {
		return "none"
	}
	if semver.Major(current) != semver.Major(latest) {
		return "major"
	}
	if semver.MajorMinor(current) != semver.MajorMinor(latest) {
		return "minor"
	}
	return "patch"
}

func (u *Updater) SetStagingDir(dir string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stagingDir = dir
}

func (u *Updater) GetStagingDir() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stagingDir
}

// begin marks the updater busy; it fails if a download or install is
// already running, so concurrent calls cannot clobber each other's staging
// directory.
func (u *Updater) begin() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busy {
		return fmt.Errorf("an update download or install is already in progress")
	}
	u.busy = true
	return nil
}

func (u *Updater) end() {
	u.mu.Lock()
	u.busy = false
	u.mu.Unlock()
}

func (u *Updater) CheckForUpdates() (*UpdateInfo, error) {
	info := &UpdateInfo{
		CurrentVersion: u.currentVersion,
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", u.apiBase, repoOwner, repoName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("check updates: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("updates temporarily unavailable (rate limit); please try again later")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found for this repository")
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("github API unavailable (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response from GitHub API (HTTP %d)", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing release data: %w", err)
	}

	info.LatestVersion = release.TagName
	info.ReleaseURL = release.HTMLURL
	info.ReleaseNotes = release.Body

	if release.TagName == "" {
		return nil, fmt.Errorf("release has no version tag")
	}
	if len(release.Assets) == 0 {
		return nil, fmt.Errorf("no assets found in the latest release")
	}

	current := u.currentVersion
	latest := release.TagName
	info.HasUpdate = versionIsNewer(current, latest)
	info.Severity = versionBump(current, latest)

	asset, err := resolveAsset(release.Assets)
	if err != nil {
		return nil, err
	}
	info.AssetURL = asset.BrowserDownloadURL

	// Remember the release so DownloadUpdate can fetch exactly these assets.
	var pending *pendingRelease
	if info.HasUpdate {
		pending = &pendingRelease{
			archive:   asset,
			checksums: findAsset(release.Assets, checksumsAsset),
			signature: findAsset(release.Assets, signatureAsset),
		}
	}
	u.mu.Lock()
	u.pending = pending
	u.mu.Unlock()

	return info, nil
}

func resolveAsset(assets []githubAsset) (githubAsset, error) {
	pk := platformKey()
	ext := archiveExt()

	for _, a := range assets {
		name := strings.ToLower(a.Name)
		if !strings.Contains(name, pk) {
			continue
		}
		if strings.HasSuffix(name, ext) {
			return a, nil
		}
	}

	return githubAsset{}, fmt.Errorf("update not supported on this platform (%s/%s)", runtime.GOOS, runtime.GOARCH)
}

// findAsset returns the asset with exactly the given name, or nil.
func findAsset(assets []githubAsset, name string) *githubAsset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

// PrepareUpdate downloads, verifies and extracts the release found by the
// last CheckForUpdates into a fresh staging directory (replacing any previous
// one), ready for InstallStaged. Concurrent calls are rejected.
func (u *Updater) PrepareUpdate(ctx context.Context, onProgress func(downloaded, total int64)) error {
	if err := u.begin(); err != nil {
		return err
	}
	defer u.end()

	// Clean up any staging dir from a previous download attempt so repeated
	// downloads don't leak temp directories.
	if prev := u.GetStagingDir(); prev != "" {
		os.RemoveAll(prev)
		u.SetStagingDir("")
	}

	archivePath, err := u.DownloadUpdate(ctx, onProgress)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	stagingDir, err := os.MkdirTemp("", "papeer-update")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	if _, err := u.ExtractArchive(archivePath, stagingDir); err != nil {
		os.RemoveAll(stagingDir)
		return err
	}

	u.SetStagingDir(stagingDir)
	return nil
}

// InstallStaged installs the update prepared by PrepareUpdate and restarts.
// It refuses to run while a download is still in progress.
func (u *Updater) InstallStaged() error {
	if err := u.begin(); err != nil {
		return err
	}
	// Only reached when the install fails; on success the process exits.
	defer u.end()

	stagingDir := u.GetStagingDir()
	if stagingDir == "" {
		return fmt.Errorf("no downloaded update to install")
	}
	return u.InstallAndRestart(stagingDir)
}

// DownloadUpdate downloads the archive of the release found by the last
// CheckForUpdates, verifies it against the release's SHA256SUMS and returns
// the path of the verified archive (a temp file the caller must remove).
// The archive is deleted on any verification failure.
func (u *Updater) DownloadUpdate(ctx context.Context, onProgress func(downloaded, total int64)) (string, error) {
	u.mu.Lock()
	rel := u.pending
	u.mu.Unlock()
	if rel == nil {
		return "", fmt.Errorf("no update available to download; check for updates first")
	}

	want, err := u.expectedChecksum(ctx, rel)
	if err != nil {
		return "", err
	}

	path, got, err := u.downloadAsset(ctx, rel.archive, onProgress)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(got, want) {
		os.Remove(path)
		return "", fmt.Errorf("checksum mismatch for %s: update rejected", rel.archive.Name)
	}
	return path, nil
}

// expectedChecksum fetches the release's SHA256SUMS and returns the digest
// listed for the archive. Verification policy:
//
//   - SHA256SUMS must be published with the release and list the archive by
//     name; otherwise the update is refused (never install unverified bytes).
//   - When PublicKey is set (release builds), SHA256SUMS.sig must also be
//     present and be a valid ed25519 signature of SHA256SUMS under that key.
//   - When PublicKey is empty (dev builds, or before the maintainer has
//     configured a signing key), the signature check is skipped with a logged
//     warning: the archive hash still protects integrity, but not
//     authenticity.
func (u *Updater) expectedChecksum(ctx context.Context, rel *pendingRelease) ([]byte, error) {
	if rel.checksums == nil {
		return nil, fmt.Errorf("release has no %s; refusing to install an unverified update", checksumsAsset)
	}
	sums, err := u.fetchSmall(ctx, rel.checksums.BrowserDownloadURL)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", checksumsAsset, err)
	}

	if PublicKey != "" {
		if rel.signature == nil {
			return nil, fmt.Errorf("release has no %s; refusing to install an unsigned update", signatureAsset)
		}
		sig, err := u.fetchSmall(ctx, rel.signature.BrowserDownloadURL)
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", signatureAsset, err)
		}
		if err := VerifySignature(PublicKey, sums, sig); err != nil {
			return nil, fmt.Errorf("%s: %w", checksumsAsset, err)
		}
	} else {
		log.Printf("updater: WARNING: no update signing key built in; skipping %s signature check (hash-only verification)", checksumsAsset)
	}

	parsed, err := parseChecksums(sums)
	if err != nil {
		return nil, err
	}
	want, ok := parsed[rel.archive.Name]
	if !ok {
		return nil, fmt.Errorf("%s has no entry for %s", checksumsAsset, rel.archive.Name)
	}
	return want, nil
}

// get issues an allowlisted GET on the download client and checks for 200.
func (u *Updater) get(ctx context.Context, rawURL string) (*http.Response, error) {
	if err := u.checkAssetURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := u.dlClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download failed (HTTP %d)", resp.StatusCode)
	}
	return resp, nil
}

// fetchSmall downloads a small release asset (checksums, signature) into
// memory, capped at maxChecksumsBytes.
func (u *Updater) fetchSmall(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := u.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumsBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxChecksumsBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxChecksumsBytes)
	}
	return data, nil
}

// downloadAsset streams the archive to a temp file, enforcing the size cap
// (the API-advertised size when known, never more than maxDownloadBytes),
// and returns the file path together with its SHA-256.
func (u *Updater) downloadAsset(ctx context.Context, asset githubAsset, onProgress func(downloaded, total int64)) (string, []byte, error) {
	limit := int64(maxDownloadBytes)
	if asset.Size > 0 && asset.Size < limit {
		limit = asset.Size
	}

	resp, err := u.get(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return "", nil, fmt.Errorf("download update: %w", err)
	}
	defer resp.Body.Close()

	total := resp.ContentLength
	if total > limit {
		return "", nil, fmt.Errorf("download too large: %d bytes (limit %d)", total, limit)
	}

	ext := archiveExt()
	tmpFile, err := os.CreateTemp("", "papeer-update-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	fail := func(err error) (string, []byte, error) {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", nil, err
	}

	hash := sha256.New()
	body := io.LimitReader(resp.Body, limit+1)
	var downloaded int64
	buf := make([]byte, 32*1024)
	lastEmit := int64(0)

	for {
		nr, readErr := body.Read(buf)
		if nr > 0 {
			if downloaded+int64(nr) > limit {
				return fail(fmt.Errorf("download exceeds the expected size of %d bytes", limit))
			}
			nw, writeErr := tmpFile.Write(buf[:nr])
			if writeErr != nil {
				return fail(fmt.Errorf("write download: %w", writeErr))
			}
			hash.Write(buf[:nw])
			downloaded += int64(nw)

			if onProgress != nil && (downloaded-lastEmit >= 1024*1024 || downloaded == total) {
				onProgress(downloaded, total)
				lastEmit = downloaded
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fail(fmt.Errorf("download interrupted: %w", readErr))
		}
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}

	if total > 0 && downloaded != total {
		os.Remove(tmpFile.Name())
		return "", nil, fmt.Errorf("download incomplete: expected %d bytes, got %d", total, downloaded)
	}

	return tmpFile.Name(), hash.Sum(nil), nil
}

// ExtractArchive unpacks the archive into destDir and returns the path of the
// executable found in it. All writes go through an os.Root anchored at
// destDir, so no entry — including one addressed through a symlink created by
// an earlier entry — can land outside destDir.
func (u *Updater) ExtractArchive(archivePath, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return "", fmt.Errorf("open staging dir: %w", err)
	}
	defer root.Close()

	if strings.HasSuffix(archivePath, ".tar.gz") {
		err = extractTarGz(archivePath, root)
	} else {
		err = extractZip(archivePath, root)
	}
	if err != nil {
		return "", err
	}

	binaryPath := findExtractedBinary(destDir)
	if binaryPath == "" {
		return "", fmt.Errorf("executable binary not found in archive")
	}

	return binaryPath, nil
}

// extractBudget caps what an archive may expand to (decompression bombs).
type extractBudget struct {
	entries int
	bytes   int64
}

func (b *extractBudget) addEntry() error {
	b.entries++
	if b.entries > maxExtractEntries {
		return fmt.Errorf("archive has too many entries (limit %d)", maxExtractEntries)
	}
	return nil
}

// copy copies r into w, charging the bytes against the total size budget.
func (b *extractBudget) copy(w io.Writer, r io.Reader) error {
	n, err := io.Copy(w, io.LimitReader(r, maxExtractBytes-b.bytes+1))
	b.bytes += n
	if err != nil {
		return err
	}
	if b.bytes > maxExtractBytes {
		return fmt.Errorf("archive expands beyond %d bytes", maxExtractBytes)
	}
	return nil
}

// archiveEntryPath validates an archive entry name (zip-slip) and returns it
// as a clean, local, OS-specific relative path.
func archiveEntryPath(name string) (string, error) {
	p := filepath.Clean(filepath.FromSlash(name))
	if name == "" || !filepath.IsLocal(p) {
		return "", fmt.Errorf("invalid path in archive: %s", name)
	}
	return p, nil
}

// checkSymlinkTarget rejects symlinks that are absolute or whose target,
// resolved relative to the link's directory, escapes the extraction root.
func checkSymlinkTarget(entry, target string) error {
	t := filepath.FromSlash(target)
	if target == "" || filepath.IsAbs(t) || filepath.VolumeName(t) != "" ||
		strings.HasPrefix(target, "/") || strings.HasPrefix(target, `\`) ||
		!filepath.IsLocal(filepath.Join(filepath.Dir(entry), t)) {
		return fmt.Errorf("invalid symlink in archive: %s -> %s", entry, target)
	}
	return nil
}

// createFile creates name inside root for writing. Any existing entry of that
// name (e.g. a symlink from an earlier archive entry) is removed first, so the
// write never follows a link.
func createFile(root *os.Root, name string, mode os.FileMode) (*os.File, error) {
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	_ = root.Remove(name)
	return root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
}

func extractZip(zipPath string, root *os.Root) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()

	var budget extractBudget
	for _, f := range reader.File {
		if err := budget.addEntry(); err != nil {
			return err
		}
		if err := extractZipFile(f, root, &budget); err != nil {
			return err
		}
	}
	return nil
}

// findExtractedBinary locates the executable inside an extracted archive.
// For a macOS .app bundle it looks under Contents/MacOS; otherwise (Windows
// .exe, Linux binary) it returns the first valid top-level regular file. This
// avoids mistaking Info.plist or a resource file for the executable.
func findExtractedBinary(destDir string) string {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return ""
	}

	// macOS: look inside a .app bundle first.
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			macOSDir := filepath.Join(destDir, e.Name(), "Contents", "MacOS")
			binEntries, err := os.ReadDir(macOSDir)
			if err != nil {
				continue
			}
			for _, b := range binEntries {
				if cand := validateBinary(filepath.Join(macOSDir, b.Name())); cand != "" {
					return cand
				}
			}
		}
	}

	// Flat archive: first valid regular file at the top level.
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if cand := validateBinary(filepath.Join(destDir, e.Name())); cand != "" {
			return cand
		}
	}

	return ""
}

func extractZipFile(f *zip.File, root *os.Root, budget *extractBudget) error {
	name, err := archiveEntryPath(f.Name)
	if err != nil {
		return err
	}

	if f.FileInfo().IsDir() {
		return root.MkdirAll(name, 0o755)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	// Recreate symlinks as symlinks (macOS .app frameworks use them); the
	// entry's content is the link target. Only links that stay inside the
	// extraction root are allowed.
	if f.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := io.ReadAll(io.LimitReader(rc, maxSymlinkTarget+1))
		if err != nil {
			return err
		}
		if len(linkTarget) > maxSymlinkTarget {
			return fmt.Errorf("invalid symlink in archive: %s", f.Name)
		}
		if err := checkSymlinkTarget(name, string(linkTarget)); err != nil {
			return err
		}
		if dir := filepath.Dir(name); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		_ = root.Remove(name)
		return root.Symlink(string(linkTarget), name)
	}

	// Preserve the archived file mode so executables keep their +x bit.
	mode := f.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := createFile(root, name, mode)
	if err != nil {
		return err
	}
	if err := budget.copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func extractTarGz(tgzPath string, root *os.Root) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return fmt.Errorf("open tar.gz: %w", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	var budget extractBudget
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}
		if err := budget.addEntry(); err != nil {
			return err
		}

		name, err := archiveEntryPath(header.Name)
		if err != nil {
			return err
		}

		// Other entry types (symlinks, devices, …) are skipped: the Linux
		// release tarball only carries the bare binary.
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			mode := os.FileMode(header.Mode).Perm()
			if mode == 0 {
				mode = 0o644
			}
			out, err := createFile(root, name, mode)
			if err != nil {
				return err
			}
			if err := budget.copy(out, tarReader); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBinary(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return ""
	}
	if runtime.GOOS != "windows" {
		return path
	}
	if strings.HasSuffix(strings.ToLower(path), ".exe") {
		return path
	}
	return ""
}

func (u *Updater) InstallAndRestart(stagingDir string) error {
	switch runtime.GOOS {
	case "darwin":
		return installMacOS(stagingDir)
	case "windows":
		return installWindows(stagingDir)
	case "linux":
		return installLinux(stagingDir)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func findAppBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}

	// Walk up from <app>/Contents/MacOS/<binary> to find the .app bundle.
	dir := exe
	for {
		if strings.HasSuffix(dir, ".app") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find .app bundle from %s", exe)
		}
		dir = parent
	}
}

func installMacOS(stagingDir string) error {
	oldApp, err := findAppBundle()
	if err != nil {
		return err
	}

	// Find the new .app in staging.
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return fmt.Errorf("read staging dir: %w", err)
	}
	var newApp string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			newApp = filepath.Join(stagingDir, e.Name())
			break
		}
	}
	if newApp == "" {
		return fmt.Errorf("no .app bundle found in staging directory")
	}

	// Find the new binary to launch.
	newBinary := filepath.Join(newApp, "Contents", "MacOS")
	binEntries, err := os.ReadDir(newBinary)
	if err != nil || len(binEntries) == 0 {
		return fmt.Errorf("no executable found in new .app bundle")
	}
	newBinary = filepath.Join(newBinary, binEntries[0].Name())

	if info, err := os.Stat(newBinary); err != nil || info.Size() == 0 {
		return fmt.Errorf("new binary is missing or empty: %w", err)
	}

	// Move the old bundle aside to a sibling path on the SAME volume, then
	// install the new one in its place. A sibling backup avoids ~/.Trash name
	// clashes (a leftover Papeer.app from a prior update makes os.Rename fail
	// with EEXIST) and cross-volume EXDEV when the app lives on an external
	// disk. The backup is removed once the swap succeeds.
	backupPath := fmt.Sprintf("%s.old-%d", oldApp, os.Getpid())
	_ = os.RemoveAll(backupPath)
	if err := os.Rename(oldApp, backupPath); err != nil {
		return fmt.Errorf("move old app aside: %w — check permissions", err)
	}

	// Copy the new .app into the original location.
	if err := copyDir(newApp, oldApp); err != nil {
		os.RemoveAll(oldApp)
		os.Rename(backupPath, oldApp) // restore the original app
		return fmt.Errorf("copy new app: %w — check permissions", err)
	}

	// Integrity check after placement: verify the installed binary before
	// launching. If it's missing/empty, roll back to the original app.
	targetBinary := filepath.Join(oldApp, "Contents", "MacOS", filepath.Base(newBinary))
	if info, err := os.Stat(targetBinary); err != nil || info.Size() == 0 {
		os.RemoveAll(oldApp)
		os.Rename(backupPath, oldApp) // restore the original app
		return fmt.Errorf("installed app failed verification")
	}

	// Ensure the installed binary is executable (some archives drop the +x bit).
	os.Chmod(targetBinary, 0o755)

	// Swap succeeded — drop the backup (best effort; a leftover is harmless).
	os.RemoveAll(backupPath)

	// Relaunch through LaunchServices (`open`) so the new instance gets a
	// proper GUI session and appears in the Dock. Fall back to exec'ing the
	// binary directly. Only exit once relaunch is under way; if both fail,
	// return the error and keep the current process alive so the user isn't
	// left with nothing.
	if err := exec.Command("open", "-n", oldApp).Run(); err != nil {
		cmd := exec.Command(targetBinary)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if startErr := cmd.Start(); startErr != nil {
			return fmt.Errorf("relaunch failed (open: %v; exec: %w)", err, startErr)
		}
	}

	os.Exit(0)
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		// Preserve symlinks (filepath.Walk uses Lstat, so ModeSymlink is set).
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			os.Remove(target)
			return os.Symlink(linkTarget, target)
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

// psQuote returns s as a single-quoted PowerShell string literal with any
// embedded single quotes doubled.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func installWindows(stagingDir string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return err
	}

	// Find the new .exe in staging.
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return fmt.Errorf("read staging dir: %w", err)
	}
	var newExe string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".exe") {
			newExe = filepath.Join(stagingDir, e.Name())
			break
		}
	}
	if newExe == "" {
		return fmt.Errorf("no .exe found in staging directory")
	}

	if info, err := os.Stat(newExe); err != nil || info.Size() == 0 {
		return fmt.Errorf("new binary is missing or empty: %w", err)
	}

	// A unique temp file (not a fixed name another user/process could
	// pre-create or race on).
	f, err := os.CreateTemp("", "papeer-update-*.ps1")
	if err != nil {
		return fmt.Errorf("create update script: %w", err)
	}
	scriptPath := f.Name()
	_, err = f.WriteString(windowsUpdateScript(os.Getpid(), newExe, exePath))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("write update script: %w", err)
	}

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	if err := cmd.Start(); err != nil {
		// Keep the current process alive rather than exiting into nothing.
		os.Remove(scriptPath)
		return fmt.Errorf("start update script: %w", err)
	}

	os.Exit(0)
	return nil
}

// windowsUpdateScript builds the PowerShell script that waits for the
// running process (pid) to exit, copies newExe over exePath (retrying while
// the file is still locked) and relaunches exePath — the new binary on
// success, the untouched old one otherwise.
//
// Paths are emitted as single-quoted PowerShell literals (no interpolation)
// with embedded quotes doubled, so a stray character in a path cannot break
// out of the string or inject commands.
func windowsUpdateScript(pid int, newExe, exePath string) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
try { Wait-Process -Id %d -Timeout 60 } catch { }
$copied = $false
for ($i = 0; $i -lt 10 -and -not $copied; $i++) {
  try {
    Copy-Item -LiteralPath %s -Destination %s -Force
    $copied = $true
  } catch {
    Start-Sleep -Seconds 1
  }
}
Start-Process -FilePath %s
Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue
`, pid, psQuote(newExe), psQuote(exePath), psQuote(exePath))
}

func installLinux(stagingDir string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return err
	}

	if err := replaceLinuxBinary(stagingDir, exePath); err != nil {
		return err
	}
	return syscall.Exec(exePath, os.Args, os.Environ())
}

// replaceLinuxBinary installs stagingDir/papeer over exePath, keeping the
// previous binary as exePath+".bak" so a failed swap can be rolled back.
func replaceLinuxBinary(stagingDir, exePath string) error {
	// Locate the binary by its expected name, not "first file in the dir".
	newBinary := filepath.Join(stagingDir, linuxBinaryName)
	if info, err := os.Lstat(newBinary); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("new binary %q is missing, empty or not a regular file", linuxBinaryName)
	}

	// Copy the new binary to a sibling temp file on the SAME filesystem as the
	// target, then rename it over the running binary. A direct os.Rename from
	// the OS temp dir fails with EXDEV when /tmp is a separate mount (tmpfs),
	// so staging next to the target is required for the atomic replace to work.
	// A stale .new from an earlier attempt is removed, then the file is created
	// with O_EXCL so a pre-planted file or symlink is never written through.
	tmpPath := exePath + ".new"
	os.Remove(tmpPath)
	if err := copyFileExcl(newBinary, tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("stage new binary next to target: %w — check permissions", err)
	}

	// Keep the old binary (hard link, falling back to a copy) for rollback.
	bakPath := exePath + ".bak"
	os.Remove(bakPath)
	if err := os.Link(exePath, bakPath); err != nil {
		if err := copyFileExcl(exePath, bakPath, 0o755); err != nil {
			os.Remove(tmpPath)
			os.Remove(bakPath)
			return fmt.Errorf("back up current binary: %w — check permissions", err)
		}
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		os.Remove(bakPath) // the original is still in place
		return fmt.Errorf("replace binary: %w — check permissions", err)
	}

	// Integrity check after placement, before restart; roll back on failure.
	if info, err := os.Stat(exePath); err != nil || info.Size() == 0 {
		if rbErr := os.Rename(bakPath, exePath); rbErr != nil {
			return fmt.Errorf("installed binary failed verification and rollback failed: %w", rbErr)
		}
		return fmt.Errorf("installed binary failed verification")
	}

	// The .bak is kept deliberately until the next update so a broken new
	// release can be reverted by hand.
	return nil
}

// copyFileExcl copies src to a new file dst, failing if dst already exists.
func copyFileExcl(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
