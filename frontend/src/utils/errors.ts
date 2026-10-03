import { useI18n } from 'vue-i18n'

const INVALID_EMAIL_MARKER = 'a real contact email is required'

export function isInvalidEmailError(e: unknown): boolean {
  const msg = (e as { message?: string })?.message ?? String(e)
  return msg.includes(INVALID_EMAIL_MARKER)
}

// Known raw Go error substrings, mapped to localized i18n keys. The backend
// returns plain strings over Wails RPC, so these markers must track the Go
// messages they match (grep the marker in internal/ when changing one).
// Matched in order, first hit wins — keep the more specific markers ahead of
// shorter phrases they contain (e.g. "no approved or downloaded" before
// "no approved").
const KNOWN_BACKEND_ERRORS: Array<[string, string]> = [
  // Analysis (internal/app/app_*.go)
  ['no approved or downloaded papers to export', 'v2.analysis.errors.noApprovedOrDownloaded'],
  ['no approved or downloaded papers', 'v2.analysis.errors.noApprovedOrDownloaded'],
  ['no approved papers to download', 'v2.analysis.errors.noApprovedToDownload'],
  ['no approved papers', 'v2.analysis.errors.noApproved'],
  ['citation fetch already in progress', 'v2.analysis.errors.citationFetchBusy'],
  ['review draft generation already in progress', 'v2.analysis.errors.reviewBusy'],
  ['no coverage gaps to export', 'v2.analysis.errors.noGapsToExport'],
  ['no recurring uncited works', 'v2.analysis.errors.noGapsToExport'],
  // llm.ErrKeyringUnavailable (internal/llm/keyring.go)
  ['OS keychain is unavailable', 'settings.keychainUnavailable'],
  // Downloads / PDFs
  ['PDF directory not configured', 'errors.pdfDirNotConfigured'],
  ['no failed downloads', 'errors.noFailedDownloads'],
  ['no downloaded papers with PDFs', 'errors.noDownloadedPdfs'],
  ['PDF not downloaded for this paper', 'errors.noPdf'],
  ['pdf file not found', 'errors.pdfFileMissing'],
  ['pdf storage not ready', 'errors.pdfUnavailable'],
  ['chrome/chromium binary not found', 'errors.chromeNotFound'],
  ['no valid papers found for the given IDs', 'errors.noValidPapers'],
  // Settings
  ['invalid proxy URL', 'errors.invalidProxy'],
  ['unknown setting', 'errors.unknownSetting'],
  ['updater not initialized', 'errors.updaterUnavailable'],
  // LLM profiles and summaries (internal/app/app.go, validate.go)
  ['summary generation already in progress', 'errors.summaryBusy'],
  ['summary already exists', 'errors.summaryExists'],
  ['API key not configured for the active LLM profile', 'errors.noLlmKey'],
  ['no active LLM profile', 'errors.noActiveLlmProfile'],
  ['requested model is not part of the active LLM profile', 'errors.modelNotInProfile'],
  ['no model specified for connection test', 'errors.noModel'],
  ['no model available in active profile', 'errors.noModel'],
  ['LLM base URL must use https', 'errors.insecureLlmUrl'],
  ['scheme must be https', 'errors.insecureLlmUrl'],
  ['the LLM base URL host changed', 'errors.llmKeyReentry'],
  ['invalid LLM base URL', 'errors.invalidLlmUrl'],
  // Axes import/export (internal/export/yaml.go, internal/app)
  ['yaml file is larger than', 'errors.yamlTooLarge'],
  ['no axes to export', 'errors.noAxesToExport'],
  // Network (internal/httpclient)
  ['destination not allowed', 'errors.blockedDestination'],
  ['response body too large', 'errors.bodyTooLarge'],
  ['Client.Timeout exceeded', 'errors.timeout'],
  ['context deadline exceeded', 'errors.timeout'],
  ['no such host', 'errors.network'],
  ['connection refused', 'errors.network'],
]

// errorMessage extracts the raw text of a rejected Wails call (a plain
// string) or a JS Error.
export function errorMessage(e: unknown): string {
  if (typeof e === 'string') return e
  const m = (e as { message?: unknown })?.message
  return typeof m === 'string' && m !== '' ? m : String(e)
}

// localizeBackendError returns a user-facing message for a backend error:
// a translated text for known markers, otherwise the raw message.
export function localizeBackendError(e: unknown, t: ReturnType<typeof useI18n>['t']): string {
  if (isInvalidEmailError(e)) return t('profiles.emailMissingForOps')
  const msg = errorMessage(e)
  for (const [marker, key] of KNOWN_BACKEND_ERRORS) {
    if (msg.includes(marker)) return t(key)
  }
  return msg
}

// downloadFailReason condenses a download engine failure (often the
// aggregated "all sources exhausted: src: reason; ..." string) into one short
// localized reason for compact lists. Full details stay in the download log.
export function downloadFailReason(err: string, t: ReturnType<typeof useI18n>['t']): string {
  const known = localizeBackendError(err, t)
  if (known !== errorMessage(err)) return known
  const lower = err.toLowerCase()
  if (/captcha|cloudflare|akamai|\b403\b|forbidden/.test(lower)) return t('download.failReason.blocked')
  if (/\b429\b|rate limit|too many requests/.test(lower)) return t('download.failReason.rateLimited')
  if (/not a pdf|pdf validation|truncated|html page instead of pdf|paywall/.test(lower)) return t('download.failReason.invalidPdf')
  if (lower.startsWith('all sources exhausted')) return t('download.failReason.noSource')
  return t('download.failReason.other')
}
