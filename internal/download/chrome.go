package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// ErrChromeNotFound is returned by FetchPDFWithChrome when no Chrome/Chromium
// binary is available on the host. Callers should treat it as a soft failure
// (fall through to the next source) rather than a hard error.
var ErrChromeNotFound = errors.New("chrome/chromium binary not found on system")

// chromeUA is the UA chromedp announces. Akamai BMP keys its sensor data
// against the UA string, so we leave it set to a stock Chrome value that
// matches what genuine Chrome installs send.
const chromeUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

// ChromeAvailable reports whether a Chrome/Chromium binary is reachable on
// this host. The check mirrors chromedp's own binary discovery so we can
// fail fast and skip the source instead of throwing a stack trace at the user.
func ChromeAvailable() bool {
	return findChromeBinary() != ""
}

// findChromeBinary returns the path to a Chrome/Chromium-family binary, or
// "" if none is installed.
func findChromeBinary() string {
	// Honor env override first — same variable chromedp itself respects.
	if p := os.Getenv("CHROMEDP_CHROME_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	case "windows":
		programFiles := []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("LocalAppData"),
		}
		for _, base := range programFiles {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Chromium", "Application", "chrome.exe"),
				filepath.Join(base, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			)
		}
	default: // linux, freebsd, etc.
		for _, name := range []string{
			"google-chrome", "google-chrome-stable", "chromium",
			"chromium-browser", "chrome", "microsoft-edge", "brave-browser",
		} {
			if p, err := exec.LookPath(name); err == nil {
				return p
			}
		}
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// FetchPDFWithChrome downloads a PDF via headless Chrome. The browser
// navigates to rawURL, lets the page's JS solve any bot challenge (Akamai
// BMP, Cloudflare, MDPI interstitials), and grabs the PDF bytes via the
// Fetch domain's response interception — that's the only reliable way to
// read a top-level PDF response in headless Chrome (Network.GetResponseBody
// is racy once the PDF viewer takes over).
//
// Returns ErrChromeNotFound if no Chrome binary exists — caller should treat
// that as a soft skip, not a hard failure.
func FetchPDFWithChrome(ctx context.Context, rawURL, dest string) error {
	chromePath := findChromeBinary()
	if chromePath == "" {
		return ErrChromeNotFound
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Headless,
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
		chromedp.UserAgent(chromeUA),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	timeoutCtx, cancelTimeout := context.WithTimeout(browserCtx, 90*time.Second)
	defer cancelTimeout()

	// pdfResult holds the captured PDF bytes (or an interception error).
	type pdfResult struct {
		body []byte
		err  error
	}
	resultCh := make(chan pdfResult, 1)
	var deliverOnce sync.Once
	deliver := func(r pdfResult) {
		deliverOnce.Do(func() {
			select {
			case resultCh <- r:
			default:
			}
		})
	}

	// Fetch interception: pause every response, inspect its MIME, capture
	// the body for the first PDF, continue everything else untouched. We do
	// the CDP work in goroutines to avoid blocking the event dispatcher.
	chromedp.ListenTarget(timeoutCtx, func(ev interface{}) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		go func() {
			// Detect PDF by Content-Type header or .pdf URL suffix as fallback.
			isPDF := false
			for _, h := range e.ResponseHeaders {
				if strings.EqualFold(h.Name, "content-type") &&
					strings.Contains(strings.ToLower(h.Value), "application/pdf") {
					isPDF = true
					break
				}
			}
			if !isPDF && strings.Contains(strings.ToLower(e.Request.URL), ".pdf") {
				// Heuristic: URL ends in .pdf but no Content-Type header set
				// yet. Only trust this when we already have a response stage
				// event (we do — RequestStageResponse).
				isPDF = e.ResponseStatusCode != 0
			}

			if !isPDF {
				// Continue everything else so navigation completes.
				_ = chromedp.Run(timeoutCtx,
					chromedp.ActionFunc(func(ctx context.Context) error {
						return fetch.ContinueRequest(e.RequestID).Do(ctx)
					}),
				)
				return
			}

			// Capture PDF body, then continue (so Chrome doesn't hang).
			var body []byte
			err := chromedp.Run(timeoutCtx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					b, err := fetch.GetResponseBody(e.RequestID).Do(ctx)
					if err != nil {
						return err
					}
					body = b
					return nil
				}),
			)
			// Always continue, even on error, so Chrome moves on.
			_ = chromedp.Run(timeoutCtx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					return fetch.ContinueRequest(e.RequestID).Do(ctx)
				}),
			)
			if err != nil {
				deliver(pdfResult{err: fmt.Errorf("fetch.GetResponseBody for %s: %w", e.Request.URL, err)})
				return
			}
			deliver(pdfResult{body: body})
		}()
	})

	// Run: enable network, enable fetch interception at the Response stage,
	// then navigate. Pausing at Response means headers/body are ready when
	// we receive EventRequestPaused.
	if err := chromedp.Run(timeoutCtx,
		network.Enable(),
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{
			{URLPattern: "*", RequestStage: fetch.RequestStageResponse},
		}),
	); err != nil {
		return fmt.Errorf("chrome enable fetch: %w", err)
	}

	// Navigate in a goroutine — chromedp.Navigate blocks until load event,
	// but the PDF arrives mid-load and we want to surface it as soon as the
	// interception fires.
	navErrCh := make(chan error, 1)
	go func() {
		navErrCh <- chromedp.Run(timeoutCtx, chromedp.Navigate(rawURL))
	}()

	select {
	case r := <-resultCh:
		if r.err != nil {
			return fmt.Errorf("chrome PDF capture: %w", r.err)
		}
		if err := ValidatePDF(r.body); err != nil {
			return fmt.Errorf("chrome got non-PDF from %s: %w", rawURL, err)
		}
		if err := os.WriteFile(dest, r.body, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
		return nil

	case err := <-navErrCh:
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("chrome navigate: %w", err)
		}
		// Navigation finished but no PDF was intercepted. Drain resultCh
		// briefly in case the response is in flight.
		select {
		case r := <-resultCh:
			if r.err != nil {
				return fmt.Errorf("chrome PDF capture: %w", r.err)
			}
			if err := ValidatePDF(r.body); err != nil {
				return fmt.Errorf("chrome got non-PDF from %s: %w", rawURL, err)
			}
			if err := os.WriteFile(dest, r.body, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", dest, err)
			}
			return nil
		case <-time.After(3 * time.Second):
			return fmt.Errorf("chrome navigated %s but no PDF response intercepted", rawURL)
		}

	case <-timeoutCtx.Done():
		return fmt.Errorf("chrome timeout waiting for PDF from %s", rawURL)
	}
}
