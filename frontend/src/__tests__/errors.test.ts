import { describe, it, expect } from 'vitest'
import { isInvalidEmailError, localizeBackendError, downloadFailReason, isS2RateLimited, SEARCH_SETTINGS_ROUTE } from '../utils/errors'

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

describe('localizeBackendError: general backend markers', () => {
  const cases: Array<[string, string]> = [
    ['PDF directory not configured for profile "Demo"', 'errors.pdfDirNotConfigured'],
    ['invalid proxy URL: parse "::": missing protocol scheme', 'errors.invalidProxy'],
    ['unknown setting "foo"', 'errors.unknownSetting'],
    ['OS keychain is unavailable; the API key was not saved — enter it again later', 'settings.keychainUnavailable'],
    ['summary already exists, use RegenerateSummary', 'errors.summaryExists'],
    ['no active LLM profile; add one in Settings and make it active', 'errors.noActiveLlmProfile'],
    ['invalid LLM base URL "http://x": scheme must be https', 'errors.insecureLlmUrl'],
    ['Get "https://api.x": context deadline exceeded', 'errors.timeout'],
    ['dial tcp: lookup api.x: no such host', 'errors.network'],
  ]
  for (const [raw, key] of cases) {
    it(`maps "${raw}"`, () => {
      // Wails rejects with plain strings, not Error objects.
      expect(localizeBackendError(raw, t)).toBe(key)
      expect(localizeBackendError(new Error(raw), t)).toBe(key)
    })
  }

  it('returns the raw message for unknown errors', () => {
    expect(localizeBackendError('something odd', t)).toBe('something odd')
    expect(localizeBackendError(new Error('boom'), t)).toBe('boom')
  })
})

describe('downloadFailReason', () => {
  it('classifies aggregated engine errors into short reasons', () => {
    expect(downloadFailReason('all sources exhausted: arxiv: no arxiv id; openalex: no OA location', t)).toBe('download.failReason.noSource')
    expect(downloadFailReason('all sources exhausted: publisher: captcha required', t)).toBe('download.failReason.blocked')
    expect(downloadFailReason('all sources exhausted: s2: HTTP 429', t)).toBe('download.failReason.rateLimited')
    expect(downloadFailReason('PDF validation failed for https://x: not a PDF', t)).toBe('download.failReason.invalidPdf')
    expect(downloadFailReason('weird', t)).toBe('download.failReason.other')
  })

  it('prefers known backend markers', () => {
    expect(downloadFailReason('PDF directory not configured for profile "X"', t)).toBe('errors.pdfDirNotConfigured')
  })
})

describe('isS2RateLimited', () => {
  it('matches the stable backend marker (live 429 and breaker skip)', () => {
    expect(isS2RateLimited('all sources exhausted: s2_doi: S2 rate limited: HTTP 429 from api.semanticscholar.org')).toBe(true)
    expect(isS2RateLimited('all sources exhausted: s2_title: S2 skipped: S2 rate limited (retry after 14:05:09)')).toBe(true)
  })

  it('matches raw S2 429s from older logs', () => {
    expect(isS2RateLimited('arxiv: S2 lookup failed: HTTP 429 from api.semanticscholar.org')).toBe(true)
  })

  it('ignores other hosts\' rate limits and empty values', () => {
    expect(isS2RateLimited('crossref: HTTP 429 from api.crossref.org')).toBe(false)
    expect(isS2RateLimited('')).toBe(false)
    expect(isS2RateLimited(undefined)).toBe(false)
  })

  it('downloadFailReason reports the S2 limit specifically', () => {
    const err = 'all sources exhausted: arxiv: no arXiv location in OpenAlex, S2 rate limited: HTTP 429 from api.semanticscholar.org; openalex: OpenAlex: no OA location'
    expect(downloadFailReason(err, t)).toBe('download.failReason.s2RateLimited')
    // A non-S2 429 keeps the generic rate-limit reason.
    expect(downloadFailReason('all sources exhausted: crossref: HTTP 429 from api.crossref.org', t)).toBe('download.failReason.rateLimited')
  })

  it('links to the Search settings section', () => {
    expect(SEARCH_SETTINGS_ROUTE).toEqual({ name: 'settings', query: { section: 'search' } })
  })
})
