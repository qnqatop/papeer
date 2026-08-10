package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/mod/semver"
)

const (
	repoOwner = "qnqatop"
	repoName  = "papeer"
	githubAPI = "https://api.github.com"
	userAgent = "papeer-updater"
)

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

type Updater struct {
	currentVersion string
	apiBase        string
	httpClient     *http.Client
	stagingDir     string
}

func NewUpdater(version string) *Updater {
	apiBase := githubAPI
	if envBase := os.Getenv("PAPEER_UPDATE_API"); envBase != "" {
		apiBase = envBase
	}
	return &Updater{
		currentVersion: version,
		apiBase:        apiBase,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (u *Updater) SetAPIBase(base string) {
	u.apiBase = base
}

func (u *Updater) SetHTTPClient(c *http.Client) {
	u.httpClient = c
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

func versionIsNewer(current, latest string) bool {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)
	if semver.IsValid(current) && semver.IsValid(latest) {
		return semver.Compare(latest, current) > 0
	}
	return current != latest
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
	u.stagingDir = dir
}

func (u *Updater) GetStagingDir() string {
	return u.stagingDir
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

	assetURL, err := resolveAsset(release.Assets)
	if err != nil {
		return nil, err
	}
	info.AssetURL = assetURL

	return info, nil
}

func resolveAsset(assets []githubAsset) (string, error) {
	pk := platformKey()
	ext := archiveExt()

	for _, a := range assets {
		name := strings.ToLower(a.Name)
		if !strings.Contains(name, pk) {
			continue
		}
		if strings.HasSuffix(name, ext) {
			return a.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("update not supported on this platform (%s/%s)", runtime.GOOS, runtime.GOARCH)
}

func (u *Updater) DownloadUpdate(assetURL string, onProgress func(downloaded, total int64)) (string, error) {
	req, err := http.NewRequest("GET", assetURL, nil)
	if err != nil {
		return "", fmt.Errorf("download update: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed (HTTP %d)", resp.StatusCode)
	}

	ext := archiveExt()
	tmpFile, err := os.CreateTemp("", "papeer-update-*"+ext)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	lastEmit := int64(0)

	for {
		nr, readErr := resp.Body.Read(buf)
		if nr > 0 {
			nw, writeErr := tmpFile.Write(buf[:nr])
			if writeErr != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return "", fmt.Errorf("write download: %w", writeErr)
			}
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
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("download interrupted: %w", readErr)
		}
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("close temp file: %w", err)
	}

	if total > 0 && downloaded != total {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("download incomplete: expected %d bytes, got %d", total, downloaded)
	}

	return tmpFile.Name(), nil
}

func (u *Updater) ExtractArchive(archivePath, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}

	if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGz(archivePath, destDir)
	}
	return extractZip(archivePath, destDir)
}

func extractZip(zipPath, destDir string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()

	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := extractZipFile(f, destDir); err != nil {
			return "", err
		}
	}

	binaryPath := findExtractedBinary(destDir)
	if binaryPath == "" {
		return "", fmt.Errorf("executable binary not found in archive")
	}

	return binaryPath, nil
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

func extractZipFile(f *zip.File, destDir string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	target := filepath.Join(destDir, f.Name)

	// Prevent zip-slip.
	cleanBase := filepath.Clean(destDir) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(target), cleanBase) || strings.Contains(f.Name, "..") {
		return fmt.Errorf("invalid path in archive: %s", f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	// Recreate symlinks as symlinks (macOS .app frameworks use them); the
	// entry's content is the link target.
	if f.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		os.Remove(target)
		return os.Symlink(string(linkTarget), target)
	}

	// Preserve the archived file mode so executables keep their +x bit.
	mode := f.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func extractTarGz(tgzPath, destDir string) (string, error) {
	f, err := os.Open(tgzPath)
	if err != nil {
		return "", fmt.Errorf("open tar.gz: %w", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(destDir, header.Name)
		cleanBase := filepath.Clean(destDir) + string(os.PathSeparator)
		if !strings.HasPrefix(filepath.Clean(target), cleanBase) || strings.Contains(header.Name, "..") {
			return "", fmt.Errorf("invalid path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			mode := os.FileMode(header.Mode).Perm()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tarReader); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
		}
	}

	binaryPath := findExtractedBinary(destDir)
	if binaryPath == "" {
		return "", fmt.Errorf("executable binary not found in archive")
	}

	return binaryPath, nil
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

	scriptPath := filepath.Join(os.TempDir(), "papeer-update.ps1")
	// Paths are emitted as single-quoted PowerShell literals (no interpolation)
	// with embedded quotes doubled, so a stray character in a path cannot break
	// out of the string or inject commands.
	script := fmt.Sprintf(`Start-Sleep -Seconds 3
Copy-Item -Path %s -Destination %s -Force
Start-Process -FilePath %s
Remove-Item -Path $MyInvocation.MyCommand.Path -Force
`, psQuote(newExe), psQuote(exePath), psQuote(exePath))

	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return fmt.Errorf("write update script: %w", err)
	}

	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.Start()

	os.Exit(0)
	return nil
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

	// Find the new binary in staging.
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return fmt.Errorf("read staging dir: %w", err)
	}
	var newBinary string
	for _, e := range entries {
		if !e.IsDir() {
			newBinary = filepath.Join(stagingDir, e.Name())
			break
		}
	}
	if newBinary == "" {
		return fmt.Errorf("no binary found in staging directory")
	}

	if info, err := os.Stat(newBinary); err != nil || info.Size() == 0 {
		return fmt.Errorf("new binary is missing or empty: %w", err)
	}

	// Copy the new binary to a sibling temp file on the SAME filesystem as the
	// target, then rename it over the running binary. A direct os.Rename from
	// the OS temp dir fails with EXDEV when /tmp is a separate mount (tmpfs),
	// so staging next to the target is required for the atomic replace to work.
	tmpPath := exePath + ".new"
	if err := copyFile(newBinary, tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("stage new binary next to target: %w — check permissions", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace binary: %w — check permissions", err)
	}

	// Integrity check after placement, before restart.
	if info, err := os.Stat(exePath); err != nil || info.Size() == 0 {
		return fmt.Errorf("installed binary failed verification")
	}

	return syscall.Exec(exePath, os.Args, os.Environ())
}
