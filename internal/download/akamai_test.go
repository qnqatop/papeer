package download

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/httpclient"
)

const akamaiSampleBody = `<!DOCTYPE html><html><head> <meta charset="utf-8">
<meta http-equiv="refresh" content="5; URL='/path/to/pdf?bm-verify=URLTOKEN_xyz'" />
<title>&nbsp;</title>
<script> var i = 1780515428; var j = i + Number("1527" + "00639"); </script>
</head><body>
<iframe src="/akamai/interstitial.html"></iframe>
<script>
xhr.send(JSON.stringify({"bm-verify": "JSTOKEN_abc", "pow": j}));
</script>
</body></html>`

func TestIsAkamaiInterstitial_Positive(t *testing.T) {
	if !isAkamaiInterstitial([]byte(akamaiSampleBody)) {
		t.Error("expected Akamai interstitial to be detected")
	}
}

func TestIsAkamaiInterstitial_RegularHTML(t *testing.T) {
	html := `<html><body>regular page, no challenge</body></html>`
	if isAkamaiInterstitial([]byte(html)) {
		t.Error("regular HTML should not match Akamai interstitial")
	}
}

func TestSolveAkamaiInterstitial_HappyPath(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_sec/verify" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("wrong method: %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		// Server sets the bypass cookie and tells client to reload.
		http.SetCookie(w, &http.Cookie{Name: "ak_bmsc", Value: "verified-12345"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"reload": true}`))
	}))
	defer srv.Close()

	client := httpclient.New("test@univ.edu")
	ok, err := solveAkamaiInterstitial(context.Background(), client, srv.URL+"/article/foo", []byte(akamaiSampleBody))
	if err != nil {
		t.Fatalf("solveAkamaiInterstitial err: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}

	// Payload must carry the JSON-side bm-verify (not the URL-side one) and
	// the computed pow integer.
	if got := gotPayload["bm-verify"]; got != "JSTOKEN_abc" {
		t.Errorf("bm-verify = %v, want JSTOKEN_abc", got)
	}
	// JSON unmarshals numbers as float64; pow = 1780515428 + 152700639 = 1933216067
	if got, _ := gotPayload["pow"].(float64); int64(got) != 1933216067 {
		t.Errorf("pow = %v, want 1933216067", got)
	}
}

func TestSolveAkamaiInterstitial_MissingToken(t *testing.T) {
	// Body has pow expression but no bm-verify JSON pair.
	html := `<script> var i = 1; var j = i + Number("2" + "3"); </script><body>no token</body>`
	client := httpclient.New("test@univ.edu")
	_, err := solveAkamaiInterstitial(context.Background(), client, "https://example.com/x", []byte(html))
	if err == nil || !strings.Contains(err.Error(), "bm-verify") {
		t.Errorf("expected bm-verify error, got: %v", err)
	}
}

func TestSolveAkamaiInterstitial_MissingPow(t *testing.T) {
	html := `<script>xhr.send(JSON.stringify({"bm-verify": "T", "pow": 1}));</script>`
	client := httpclient.New("test@univ.edu")
	_, err := solveAkamaiInterstitial(context.Background(), client, "https://example.com/x", []byte(html))
	if err == nil || !strings.Contains(err.Error(), "pow") {
		t.Errorf("expected pow error, got: %v", err)
	}
}
