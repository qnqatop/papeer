import { describe, it, expect } from 'vitest'
import { isInvalidEmailError, localizeBackendError } from '../utils/errors'

// localizeBackendError takes vue-i18n's `t` function; a plain passthrough
// stub is enough to verify which key/marker was matched without pulling in
// a full i18n instance.
const t = ((key: string) => key) as any

describe('isInvalidEmailError', () => {
  it('detects the backend invalid-email marker', () => {
    const err = new Error('a real contact email is required before running this operation')
    expect(isInvalidEmailError(err)).toBe(true)
  })

  it('returns false for unrelated errors', () => {
    expect(isInvalidEmailError(new Error('network timeout'))).toBe(false)
  })

  it('handles non-Error values', () => {
    expect(isInvalidEmailError('a real contact email is required')).toBe(true)
    expect(isInvalidEmailError('something else')).toBe(false)
    expect(isInvalidEmailError(undefined)).toBe(false)
  })
})

describe('localizeBackendError', () => {
  it('maps the invalid-email marker to the profiles key, ahead of anything else', () => {
    const err = new Error('a real contact email is required')
    expect(localizeBackendError(err, t)).toBe('profiles.emailMissingForOps')
  })

  it('maps known Analysis backend error substrings to i18n keys', () => {
    const cases: Array<[string, string]> = [
      ['no approved or downloaded papers to export', 'v2.analysis.errors.noApprovedOrDownloaded'],
      ['no approved or downloaded papers', 'v2.analysis.errors.noApprovedOrDownloaded'],
      ['no approved papers to download', 'v2.analysis.errors.noApprovedToDownload'],
      ['no approved papers', 'v2.analysis.errors.noApproved'],
      ['citation fetch already in progress', 'v2.analysis.errors.citationFetchBusy'],
      ['review draft generation already in progress', 'v2.analysis.errors.reviewBusy'],
      ['no coverage gaps to export', 'v2.analysis.errors.noGapsToExport'],
      ['no recurring uncited works were found', 'v2.analysis.errors.noGapsToExport'],
    ]
    for (const [raw, key] of cases) {
      expect(localizeBackendError(new Error(raw), t)).toBe(key)
    }
  })

  it('prefers the more specific "no approved or downloaded" match over the plainer "no approved" one', () => {
    // Both markers are substrings of this message; the more specific one
    // must win because it is listed first and checked in order.
    const msg = 'no approved or downloaded papers to export'
    expect(localizeBackendError(new Error(msg), t)).toBe('v2.analysis.errors.noApprovedOrDownloaded')
  })

  it('falls back to the raw message for unknown errors', () => {
    const err = new Error('some totally unrelated backend failure')
    expect(localizeBackendError(err, t)).toBe('some totally unrelated backend failure')
  })

  it('stringifies non-Error values with no message property', () => {
    expect(localizeBackendError('plain string error', t)).toBe('plain string error')
  })
})
