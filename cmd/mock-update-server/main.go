package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
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

	// Build a dummy "updated" binary.
	dummyBinary, err := buildDummyBinary()
	if err != nil {
		log.Printf("WARNING: cannot build dummy binary: %v — serving empty archive", err)
		dummyBinary = []byte("dummy")
	}

	// Create archive in the expected format.
	if runtime.GOOS == "linux" {
		assetData, err = createTarGz("papeer", dummyBinary)
	} else {
		assetData, err = createZip(assetName, dummyBinary)
	}
	if err != nil {
		log.Fatalf("create archive: %v", err)
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
	log.Println("Test steps:")
	log.Println("  1. PAPEER_UPDATE_API=http://localhost:" + port + " make dev")
	log.Println("  2. Settings → About → Check for Updates")
	log.Println("  3. Download → Extract → Restart")
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
