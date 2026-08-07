import { describe, it, expect } from 'vitest'
import { isRealEmail } from '../utils/validate'

describe('isRealEmail', () => {
  it('rejects empty / whitespace', () => {
    expect(isRealEmail('')).toBe(false)
    expect(isRealEmail('   ')).toBe(false)
  })

  it('rejects malformed addresses', () => {
    expect(isRealEmail('foo')).toBe(false)
    expect(isRealEmail('foo@')).toBe(false)
    expect(isRealEmail('@bar.com')).toBe(false)
    expect(isRealEmail('foo@bar')).toBe(false)
  })

  it('rejects placeholder domains case-insensitively', () => {
    expect(isRealEmail('foo@example.com')).toBe(false)
    expect(isRealEmail('FOO@EXAMPLE.COM')).toBe(false)
    expect(isRealEmail('x@example.org')).toBe(false)
    expect(isRealEmail('x@example.net')).toBe(false)
    expect(isRealEmail('x@test.com')).toBe(false)
    expect(isRealEmail('x@localhost')).toBe(false)
  })

  it('accepts real-looking emails', () => {
    expect(isRealEmail('a@b.co')).toBe(true)
    expect(isRealEmail('andrej.dev+research@gmail.com')).toBe(true)
    expect(isRealEmail('user@uni-bonn.de')).toBe(true)
  })
})
