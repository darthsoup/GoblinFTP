import { describe, expect, it } from 'vitest'
import { buildErrorPayload, errorDedupeKey, truncate } from '~/utils/errorReport'

describe('truncate', () => {
  it('passes undefined through', () => {
    expect(truncate(undefined, 10)).toBeUndefined()
  })

  it('keeps strings at or under the limit', () => {
    expect(truncate('short', 10)).toBe('short')
    expect(truncate('exactly10!', 10)).toBe('exactly10!')
  })

  it('cuts strings over the limit', () => {
    expect(truncate('x'.repeat(20), 10)).toBe('x'.repeat(10))
  })
})

describe('buildErrorPayload', () => {
  it('extracts message and stack from an Error', () => {
    const err = new Error('boom')
    const payload = buildErrorPayload('error', err, '/files', 'main.js:1:1')

    expect(payload.kind).toBe('error')
    expect(payload.message).toBe('boom')
    expect(payload.stack).toBeDefined()
    expect(payload.source).toBe('main.js:1:1')
    expect(payload.route).toBe('/files')
  })

  it('falls back to the error name when the message is empty', () => {
    const err = new TypeError('placeholder')
    err.message = ''
    const payload = buildErrorPayload('vue', err)
    expect(payload.message).toBe('TypeError')
  })

  it('handles thrown strings', () => {
    const payload = buildErrorPayload('rejection', 'string reason')
    expect(payload.message).toBe('string reason')
    expect(payload.stack).toBeUndefined()
  })

  it('stringifies non-Error objects', () => {
    expect(buildErrorPayload('rejection', 42).message).toBe('42')
    expect(buildErrorPayload('rejection', null).message).toBe('null')
    expect(buildErrorPayload('rejection', { a: 1 }).message).toBe('[object Object]')
  })

  it('truncates every field to the backend limits', () => {
    const err = new Error('m'.repeat(1000))
    err.stack = 's'.repeat(10_000)
    const payload = buildErrorPayload('error', err, `/r${'r'.repeat(1000)}`, 'f'.repeat(1000))

    expect(payload.message).toHaveLength(500)
    expect(payload.stack).toHaveLength(4000)
    expect(payload.route).toHaveLength(500)
    expect(payload.source).toHaveLength(500)
  })
})

describe('errorDedupeKey', () => {
  it('is stable for identical payloads', () => {
    const a = buildErrorPayload('error', new Error('boom'), '/x', 'main.js')
    const b = buildErrorPayload('error', new Error('boom'), '/x', 'main.js')
    expect(errorDedupeKey(a)).toBe(errorDedupeKey(b))
  })

  it('differs by kind, message, and source', () => {
    const base = buildErrorPayload('error', new Error('boom'), '/x', 'main.js')
    expect(errorDedupeKey(buildErrorPayload('vue', new Error('boom'), '/x', 'main.js'))).not.toBe(errorDedupeKey(base))
    expect(errorDedupeKey(buildErrorPayload('error', new Error('other'), '/x', 'main.js'))).not.toBe(errorDedupeKey(base))
    expect(errorDedupeKey(buildErrorPayload('error', new Error('boom'), '/x', 'other.js'))).not.toBe(errorDedupeKey(base))
  })

  it('does not vary by route (same error on two pages reports once)', () => {
    const a = buildErrorPayload('error', new Error('boom'), '/a')
    const b = buildErrorPayload('error', new Error('boom'), '/b')
    expect(errorDedupeKey(a)).toBe(errorDedupeKey(b))
  })
})
