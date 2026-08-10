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
	repoOwner  = "qnqatop"
	repoName   = "papeer"
	githubAPI  = "https://api.github.com"
	userAgent  = "papeer-updater"
)

type UpdateInfo struct {
	HasUpdate      bool   `json:"hasUpdate"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	AssetURL       string `json:"assetURL"`
	ReleaseURL     string `json:"releaseURL"`
	ReleaseNotes   string `json:"releaseNotes"`
}

type Updater struct {
	currentVersion string
	apiBase        string
	httpClient     *http.Client
	stagingDir     string
}

func NewUpdater(version string) *Updater {
	return &Updater{
		currentVersion: version,
		apiBase:        githubAPI,
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
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func versionIsNewer(current, latest string) bool {
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}
	if !strings.HasPrefix(latest, "v") {
		latest = "v" + latest
	}
	if semver.IsValid(current) && semver.IsValid(latest) {
		return semver.Compare(latest, current) > 0
	}
	return current != latest
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

	var binaryPath string
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := extractZipFile(f, destDir); err != nil {
			return "", err
		}
		if binaryPath == "" {
			binaryPath = validateBinary(filepath.Join(destDir, f.Name))
		}
	}

	if binaryPath == "" {
		// macOS .app bundle — find the actual executable inside.
		entries, _ := os.ReadDir(destDir)
		for _, e := range entries {
			if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
				appBinary := filepath.Join(destDir, e.Name(), "Contents", "MacOS")
				binEntries, err := os.ReadDir(appBinary)
				if err == nil && len(binEntries) > 0 {
					binaryPath = filepath.Join(appBinary, binEntries[0].Name())
					if info, err := os.Stat(binaryPath); err != nil || info.Size() == 0 {
						binaryPath = ""
					}
				}
				break
			}
		}
	}

	if binaryPath == "" {
		return "", fmt.Errorf("executable binary not found in archive")
	}

	return binaryPath, nil
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

	out, err := os.Create(target)
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
	var binaryPath string

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
			out, err := os.Create(target)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tarReader); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			if binaryPath == "" {
				binaryPath = validateBinary(target)
			}
		}
	}

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
	var appName string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			newApp = filepath.Join(stagingDir, e.Name())
			appName = e.Name()
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

	appDir := filepath.Dir(oldApp) // e.g., /Applications
	trashDir := filepath.Join(os.Getenv("HOME"), ".Trash")
	os.MkdirAll(trashDir, 0o755)

	// Move the old .app to Trash.
	trashPath := filepath.Join(trashDir, appName)
	_ = os.RemoveAll(trashPath) // Remove existing if any.
	if err := os.Rename(oldApp, trashPath); err != nil {
		return fmt.Errorf("move old app to trash: %w — check permissions", err)
	}

	// Copy the new .app to the original location.
	targetApp := filepath.Join(appDir, appName)
	if err := copyDir(newApp, targetApp); err != nil {
		// Try to restore the old app.
		os.Rename(trashPath, oldApp)
		return fmt.Errorf("copy new app: %w — check permissions", err)
	}

	// Spawn the new process.
	cmd := exec.Command(newBinary)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()

	os.Exit(0)
	return nil
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
	script := fmt.Sprintf(`Start-Sleep -Seconds 3
Copy-Item -Path "%s" -Destination "%s" -Force
Start-Process -FilePath "%s"
Remove-Item -Path $MyInvocation.MyCommand.Path -Force
`, newExe, exePath, exePath)

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

	// Make the new binary executable.
	os.Chmod(newBinary, 0o755)

	if err := os.Rename(newBinary, exePath); err != nil {
		return fmt.Errorf("replace binary: %w — check permissions", err)
	}

	return syscall.Exec(exePath, os.Args, os.Environ())
}
