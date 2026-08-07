import { useI18n } from 'vue-i18n'

const INVALID_EMAIL_MARKER = 'a real contact email is required'

export function isInvalidEmailError(e: unknown): boolean {
  const msg = (e as { message?: string })?.message ?? String(e)
  return msg.includes(INVALID_EMAIL_MARKER)
}

// Known raw Go error substrings from the Analysis backend, mapped to
// localized i18n keys. Matched in order, first hit wins — keep the more
// specific "no approved or downloaded" check ahead of the plain "no
// approved" one since the latter is a substring-adjacent phrase.
const KNOWN_ANALYSIS_ERRORS: Array<[string, string]> = [
  ['no approved or downloaded papers to export', 'v2.analysis.errors.noApprovedOrDownloaded'],
  ['no approved or downloaded papers', 'v2.analysis.errors.noApprovedOrDownloaded'],
  ['no approved papers to download', 'v2.analysis.errors.noApprovedToDownload'],
  ['no approved papers', 'v2.analysis.errors.noApproved'],
  ['citation fetch already in progress', 'v2.analysis.errors.citationFetchBusy'],
  ['review draft generation already in progress', 'v2.analysis.errors.reviewBusy'],
  ['no coverage gaps to export', 'v2.analysis.errors.noGapsToExport'],
  ['no recurring uncited works', 'v2.analysis.errors.noGapsToExport'],
]

export function localizeBackendError(e: unknown, t: ReturnType<typeof useI18n>['t']): string {
  if (isInvalidEmailError(e)) return t('profiles.emailMissingForOps')
  const msg = (e as { message?: string })?.message ?? String(e)
  for (const [marker, key] of KNOWN_ANALYSIS_ERRORS) {
    if (msg.includes(marker)) return t(key)
  }
  return msg
}
