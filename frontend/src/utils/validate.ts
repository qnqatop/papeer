const emailRe = /^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$/
const placeholderDomains = new Set([
  'example.com',
  'example.org',
  'example.net',
  'test.com',
  'localhost',
])

export function isRealEmail(value: string): boolean {
  const s = (value || '').trim().toLowerCase()
  if (!emailRe.test(s)) return false
  const host = s.slice(s.indexOf('@') + 1)
  return !placeholderDomains.has(host)
}
