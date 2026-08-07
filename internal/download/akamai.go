package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/qnqatop/papeer/internal/httpclient"
)

// Akamai Bot Manager serves an HTML interstitial that JS-solves a small
// proof-of-work and POSTs the answer to /_sec/verify. Browsers do this
// transparently; we replicate the dance so academic publishers (MDPI,
// Springer, others on Akamai) actually serve the PDF.
//
// Reference: the interstitial HTML carries:
//
//	<meta http-equiv="refresh" content="5; URL='...?...&bm-verify=TOKEN'" />
//	<script>
//	  var i = 1780515428;
//	  var j = i + Number("1527" + "00639");
//	</script>
//	xhr.send(JSON.stringify({"bm-verify": "TOKEN2", "pow": j}));

var (
	reBMVerifyJSON = regexp.MustCompile(`"bm-verify":\s*"([^"]+)"`)
	rePow          = regexp.MustCompile(`var i = (\d+);\s*var j = i \+ Number\("(\d+)"\s*\+\s*"(\d+)"\);`)
)

// isAkamaiInterstitial reports whether the HTML body is an Akamai bot
// challenge that we know how to solve.
func isAkamaiInterstitial(body []byte) bool {
	return rePow.Match(body) && reBMVerifyJSON.Match(body)
}

// solveAkamaiInterstitial POSTs the verify endpoint and returns true if
// Akamai cleared us (we'll get an updated ak_bmsc cookie). The caller should
// then re-fetch the original URL with the same client.
func solveAkamaiInterstitial(ctx context.Context, client *httpclient.Client, rawURL string, body []byte) (bool, error) {
	tokMatch := reBMVerifyJSON.FindSubmatch(body)
	if len(tokMatch) < 2 {
		return false, fmt.Errorf("no bm-verify token in body")
	}
	powMatch := rePow.FindSubmatch(body)
	if len(powMatch) < 4 {
		return false, fmt.Errorf("no pow expression in body")
	}

	i, err := strconv.ParseInt(string(powMatch[1]), 10, 64)
	if err != nil {
		return false, fmt.Errorf("parse pow i: %w", err)
	}
	concat, err := strconv.ParseInt(string(powMatch[2])+string(powMatch[3]), 10, 64)
	if err != nil {
		return false, fmt.Errorf("parse pow concat: %w", err)
	}
	powVal := i + concat

	u, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("parse target URL: %w", err)
	}
	verifyURL := fmt.Sprintf("%s://%s/_sec/verify?provider=interstitial", u.Scheme, u.Host)

	payload, _ := json.Marshal(map[string]any{
		"bm-verify": string(tokMatch[1]),
		"pow":       powVal,
	})

	headers := map[string]string{
		"Referer":        rawURL,
		"Sec-Fetch-Dest": "empty",
		"Sec-Fetch-Mode": "cors",
		"Sec-Fetch-Site": "same-origin",
		"Origin":         fmt.Sprintf("%s://%s", u.Scheme, u.Host),
	}

	respBody, status, err := client.PostJSON(ctx, verifyURL, payload, headers)
	if err != nil {
		return false, fmt.Errorf("POST verify: %w", err)
	}
	if status != 200 {
		return false, fmt.Errorf("verify returned HTTP %d: %s", status, string(respBody))
	}

	// Server replies {"reload": true} on success. Either way, the new
	// ak_bmsc cookie is on the response and is now in the jar.
	var resp map[string]any
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return false, fmt.Errorf("verify response not JSON: %s", string(respBody))
	}
	if reload, _ := resp["reload"].(bool); reload {
		return true, nil
	}
	if _, hasLoc := resp["location"]; hasLoc {
		return true, nil
	}
	return false, fmt.Errorf("verify did not return reload/location: %s", string(respBody))
}
