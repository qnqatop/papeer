//go:build !mockupdate

package updater

// defaultAPIBase returns the release API base URL and an extra origin
// (scheme://host) that asset downloads may come from in addition to the
// GitHub allowlist. Release builds always talk to GitHub over HTTPS and
// trust no extra origin; the PAPEER_UPDATE_API override exists only in
// builds tagged `mockupdate` (see apibase_mock.go).
func defaultAPIBase() (apiBase, trustedOrigin string) {
	return githubAPI, ""
}
