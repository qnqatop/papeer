//go:build mockupdate

package updater

import (
	"net/url"
	"os"
)

// defaultAPIBase honours PAPEER_UPDATE_API so scripts/test-update.sh can
// point the app at cmd/mock-update-server. This file is compiled only with
// `-tags mockupdate`; release builds never read the variable. The mock
// origin (plain http allowed) is also trusted for asset downloads.
func defaultAPIBase() (apiBase, trustedOrigin string) {
	envBase := os.Getenv("PAPEER_UPDATE_API")
	if envBase == "" {
		return githubAPI, ""
	}
	u, err := url.Parse(envBase)
	if err != nil || u.Host == "" {
		return githubAPI, ""
	}
	return envBase, u.Scheme + "://" + u.Host
}
