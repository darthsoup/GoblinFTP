import { describe, expect, it } from 'vitest'
import { basename, formatFailureLines } from '~/composables/useErrorMessage'

describe('basename', () => {
  it('returns the last path segment', () => {
    expect(basename('/a/b/c.txt')).toBe('c.txt')
    expect(basename('/a/b/')).toBe('b')
    expect(basename('solo')).toBe('solo')
  })
})

describe('formatFailureLines', () => {
  const more = (n: number) => `+${n} more`

  it('renders one "label — reason" line per item', () => {
    const out = formatFailureLines(
      [{ label: 'a', reason: 'nope' }, { label: 'b', reason: 'denied' }],
      5,
      more,
    )
    expect(out).toBe('a — nope\nb — denied')
  })

  it('caps the list and appends "+N more"', () => {
    const items = Array.from({ length: 7 }, (_, i) => ({ label: `f${i}`, reason: 'x' }))
    const out = formatFailureLines(items, 3, more)
    expect(out.split('\n')).toEqual(['f0 — x', 'f1 — x', 'f2 — x', '+4 more'])
  })
})
