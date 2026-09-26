package download

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/qnqatop/papeer/internal/httpclient"
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

// chromeProxy is the proxy Chrome routes through (same setting as the Go
// client). Guarded by chromeProxyMu since downloads run concurrently.
var (
	chromeProxyMu sync.RWMutex
	chromeProxy   string
)

// SetChromeProxy sets the proxy ("http://host:port", "socks5://host:port")
// used by FetchPDFWithChrome. An empty string means a direct connection.
// Credentials in the URL are dropped — Chrome's --proxy-server can't use them.
func SetChromeProxy(proxyURL string) {
	p := ""
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil && u.Host != "" {
			p = u.Scheme + "://" + u.Host
		}
	}
	chromeProxyMu.Lock()
	chromeProxy = p
	chromeProxyMu.Unlock()
}

func currentChromeProxy() string {
	chromeProxyMu.RLock()
	defer chromeProxyMu.RUnlock()
	return chromeProxy
}

// chromeRequestAllowed reports whether headless Chrome may issue a request
// to rawURL: http(s) only, and never to localhost or a literal
// loopback/private/link-local/unspecified/multicast IP. The navigation target
// comes from remote API responses, so it must not reach local services.
func chromeRequestAllowed(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return !httpclient.IsDisallowedHost(u.Hostname())
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
	if !chromeRequestAllowed(rawURL) {
		return fmt.Errorf("chrome: refusing to navigate to %s", rawURL)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent(chromeUA),
	)
	// Chrome refuses to start its sandbox as root on Linux (containers, CI).
	// Only there do we fall back to --no-sandbox; everywhere else the sandbox
	// stays on since the browser renders arbitrary remote pages.
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		opts = append(opts, chromedp.NoSandbox)
	}
	if p := currentChromeProxy(); p != "" {
		opts = append(opts, chromedp.ProxyServer(p))
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	timeoutCtx, cancelTimeout := context.WithTimeout(browserCtx, 90*time.Second)
	defer cancelTimeout()

	// resultCh receives the first intercepted body that validates as a PDF.
	// Rejected candidates (HTML behind a .pdf URL, stubs) only update
	// lastReject so the timeout error can explain what was seen.
	resultCh := make(chan []byte, 1)
	var (
		rejectMu   sync.Mutex
		lastReject string
	)
	reject := func(msg string) {
		rejectMu.Lock()
		lastReject = msg
		rejectMu.Unlock()
	}
	rejectReason := func() string {
		rejectMu.Lock()
		defer rejectMu.Unlock()
		if lastReject == "" {
			return ""
		}
		return " (last candidate: " + lastReject + ")"
	}

	continueReq := func(id fetch.RequestID) {
		_ = chromedp.Run(timeoutCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return fetch.ContinueRequest(id).Do(ctx)
			}),
		)
	}

	// Fetch interception: requests are paused at the request stage (to block
	// disallowed destinations, including redirect hops) and at the response
	// stage (to capture the PDF body). We do the CDP work in goroutines to
	// avoid blocking the event dispatcher.
	chromedp.ListenTarget(timeoutCtx, func(ev interface{}) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		go func() {
			responseStage := e.ResponseStatusCode != 0 || e.ResponseErrorReason != ""
			if !responseStage {
				if !chromeRequestAllowed(e.Request.URL) {
					_ = chromedp.Run(timeoutCtx,
						chromedp.ActionFunc(func(ctx context.Context) error {
							return fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(ctx)
						}),
					)
					return
				}
				continueReq(e.RequestID)
				return
			}

			// Redirects and errors are never the PDF itself.
			if e.ResponseErrorReason != "" || (e.ResponseStatusCode >= 300 && e.ResponseStatusCode < 400) {
				continueReq(e.RequestID)
				return
			}

			// Detect PDF by Content-Type header or .pdf URL as fallback; the
			// body is checked for the %PDF- magic below either way.
			isPDF := strings.Contains(strings.ToLower(e.Request.URL), ".pdf")
			var contentLength int64 = -1
			for _, h := range e.ResponseHeaders {
				switch {
				case strings.EqualFold(h.Name, "content-type"):
					if strings.Contains(strings.ToLower(h.Value), "application/pdf") {
						isPDF = true
					}
				case strings.EqualFold(h.Name, "content-length"):
					if n, err := strconv.ParseInt(strings.TrimSpace(h.Value), 10, 64); err == nil {
						contentLength = n
					}
				}
			}
			if !isPDF {
				// Continue everything else so navigation completes.
				continueReq(e.RequestID)
				return
			}
			if contentLength > httpclient.MaxDownloadSize {
				reject(fmt.Sprintf("%s too large (%d bytes)", e.Request.URL, contentLength))
				continueReq(e.RequestID)
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
			continueReq(e.RequestID)
			switch {
			case err != nil:
				reject(fmt.Sprintf("fetch.GetResponseBody for %s: %v", e.Request.URL, err))
			case int64(len(body)) > httpclient.MaxDownloadSize:
				reject(fmt.Sprintf("%s too large (%d bytes)", e.Request.URL, len(body)))
			default:
				if verr := ValidatePDF(body); verr != nil {
					reject(fmt.Sprintf("%s: %v", e.Request.URL, verr))
					return
				}
				select {
				case resultCh <- body:
				default: // another candidate already won
				}
			}
		}()
	})

	// Run: enable network, enable fetch interception at both stages, then
	// navigate. Pausing at Response means headers/body are ready when we
	// receive EventRequestPaused.
	if err := chromedp.Run(timeoutCtx,
		network.Enable(),
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{
			{URLPattern: "*", RequestStage: fetch.RequestStageRequest},
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

	save := func(body []byte) error {
		if err := writeFileAtomic(dest, body); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
		return nil
	}

	select {
	case body := <-resultCh:
		return save(body)

	case err := <-navErrCh:
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("chrome navigate: %w", err)
		}
		// Navigation finished but no PDF was intercepted. Wait briefly in
		// case the response is in flight.
		select {
		case body := <-resultCh:
			return save(body)
		case <-time.After(3 * time.Second):
			return fmt.Errorf("chrome navigated %s but no PDF response intercepted%s", rawURL, rejectReason())
		}

	case <-timeoutCtx.Done():
		return fmt.Errorf("chrome timeout waiting for PDF from %s%s", rawURL, rejectReason())
	}
}
