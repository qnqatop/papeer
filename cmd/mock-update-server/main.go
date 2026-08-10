package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

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

func main() {
	port := "19999"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	var assetName, assetURL string
	var assetData []byte
	var err error

	switch runtime.GOOS {
	case "darwin":
		assetName = "papeer-macos-universal.zip"
		assetURL = "/dl/macos.zip"
	case "windows":
		assetName = "papeer-windows-amd64.zip"
		assetURL = "/dl/windows.zip"
	case "linux":
		assetName = "papeer-linux-amd64.tar.gz"
		assetURL = "/dl/linux.tar.gz"
	}

	// Package the REAL built app so the update installs a working, launchable
	// bundle (a visible end-to-end test). Point PAPEER_REAL_APP at a .app
	// (macOS) or a binary (Linux/Windows). It MUST live outside the install
	// target (build/bin/Papeer.app) — otherwise the updater overwrites the very
	// bundle we serve and corrupts it. Fall back to a throwaway dummy otherwise.
	realPath := os.Getenv("PAPEER_REAL_APP")

	if realPath != "" {
		log.Printf("Packaging REAL app from %s", realPath)
		if runtime.GOOS == "darwin" {
			assetData, err = zipApp(realPath)
		} else {
			payload, readErr := os.ReadFile(realPath)
			if readErr != nil {
				log.Fatalf("read real binary: %v", readErr)
			}
			if runtime.GOOS == "linux" {
				assetData, err = createTarGz("papeer", payload)
			} else {
				assetData, err = createZip(assetName, payload)
			}
		}
		if err != nil {
			log.Fatalf("package real app: %v", err)
		}
	} else {
		log.Printf("No real app found — serving a throwaway dummy (won't visibly relaunch on macOS)")
		dummyBinary, buildErr := buildDummyBinary()
		if buildErr != nil {
			log.Printf("WARNING: cannot build dummy binary: %v — serving empty archive", buildErr)
			dummyBinary = []byte("dummy")
		}
		if runtime.GOOS == "linux" {
			assetData, err = createTarGz("papeer", dummyBinary)
		} else {
			assetData, err = createZip(assetName, dummyBinary)
		}
		if err != nil {
			log.Fatalf("create archive: %v", err)
		}
	}

	release := githubRelease{
		TagName: "v99.0.0",
		HTMLURL: "http://localhost:" + port + "/release",
		Body:    "## Test Release\n\nThis is a mock release for testing the auto-updater.\n\n### Changes\n- Everything is new!\n- Auto-updater works\n",
		Assets: []githubAsset{
			{Name: assetName, BrowserDownloadURL: "http://localhost:" + port + assetURL, Size: int64(len(assetData))},
		},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/repos/qnqatop/papeer/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("→ GET %s", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	})

	mux.HandleFunc(assetURL, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("→ GET %s (%d bytes)", r.URL.Path, len(assetData))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(assetData)))
		w.Write(assetData)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("→ 404 %s", r.URL.Path)
		w.WriteHeader(404)
	})

	log.Printf("Mock update server listening on http://localhost:%s", port)
	log.Printf("  Release:  http://localhost:%s/repos/qnqatop/papeer/releases/latest", port)
	log.Printf("  Download: http://localhost:%s%s", port, assetURL)
	log.Printf("  Tag:      %s (current version must be < %s to trigger update)", release.TagName, release.TagName)
	log.Println()
	log.Println("Tip: use scripts/test-update.sh for the full macOS flow, or set")
	log.Println("  PAPEER_REAL_APP=/path/to/NewApp.app (outside build/bin) to serve a")
	log.Println("  real, launchable bundle, then run papeer with")
	log.Println("  PAPEER_UPDATE_API=http://localhost:" + port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func buildDummyBinary() ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "papeer-dummy-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	src := filepath.Join(tmpDir, "main.go")
	code := `package main
import "fmt"
import "os"
func main() {
	fmt.Println("Papeer updated to v99.0.0!")
	fmt.Println("Auto-updater test completed.")
	fmt.Println("Press Enter to exit...")
	var s string
	fmt.Scanln(&s)
	os.Exit(0)
}
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		return nil, err
	}

	out := filepath.Join(tmpDir, "dummy")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go build dummy: %w", err)
	}

	return os.ReadFile(out)
}

// zipApp packages a macOS .app bundle into a zip with the bundle name as the
// top-level prefix (like `ditto -c -k --keepParent`), preserving file modes
// and symlinks so the extracted app stays launchable.
func zipApp(appPath string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	root := filepath.Dir(appPath)

	err := filepath.Walk(appPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if info.IsDir() {
			_, err := zw.Create(rel + "/")
			return err
		}

		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = rel
		hdr.Method = zip.Deflate

		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, err = w.Write([]byte(link))
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func createZip(assetName string, binary []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// For macOS, create the .app bundle structure.
	if runtime.GOOS == "darwin" {
		// Papeer.app/
		zw.Create("Papeer.app/")
		zw.Create("Papeer.app/Contents/")
		zw.Create("Papeer.app/Contents/MacOS/")
		w, _ := zw.Create("Papeer.app/Contents/MacOS/papeer")
		w.Write(binary)
	} else {
		// Windows: just papeer.exe
		name := "papeer.exe"
		if strings.HasSuffix(assetName, "papeer") {
			name = "papeer"
		}
		w, _ := zw.Create(name)
		w.Write(binary)
	}

	zw.Close()
	return buf.Bytes(), nil
}

func createTarGz(name string, binary []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	header := &tar.Header{
		Name: name,
		Size: int64(len(binary)),
		Mode: 0o755,
	}
	tw.WriteHeader(header)
	tw.Write(binary)

	tw.Close()
	gw.Close()
	return buf.Bytes(), nil
}
