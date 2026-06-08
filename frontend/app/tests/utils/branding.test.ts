import { describe, expect, it } from 'vitest'
import { brandRamp } from '~/utils/branding'

describe('brandRamp', () => {
  it('anchors the supplied color at shade 500', () => {
    const ramp = brandRamp('#2563eb')
    expect(ramp).not.toBeNull()
    expect(ramp!['500']).toBe('#2563eb')
  })

  it('produces a full 11-stop scale of valid hex colors', () => {
    const ramp = brandRamp('#2563eb')!
    const keys = ['50', '100', '200', '300', '400', '500', '600', '700', '800', '900', '950']
    expect(Object.keys(ramp)).toEqual(keys)
    for (const k of keys)
      expect(ramp[k as keyof typeof ramp]).toMatch(/^#[0-9a-f]{6}$/)
  })

  it('ramps from light to dark (50 is lightest, 950 is darkest)', () => {
    const ramp = brandRamp('#2563eb')!
    const lum = (hex: string) => Number.parseInt(hex.slice(1, 3), 16) + Number.parseInt(hex.slice(3, 5), 16) + Number.parseInt(hex.slice(5, 7), 16)
    expect(lum(ramp['50'])).toBeGreaterThan(lum(ramp['500']))
    expect(lum(ramp['500'])).toBeGreaterThan(lum(ramp['950']))
    expect(lum(ramp['400'])).toBeGreaterThan(lum(ramp['700']))
  })

  it('accepts #rgb shorthand', () => {
    const ramp = brandRamp('#f00')
    expect(ramp).not.toBeNull()
    expect(ramp!['500']).toBe('#ff0000')
  })

  it('returns null for malformed input', () => {
    expect(brandRamp('blue')).toBeNull()
    expect(brandRamp('#12')).toBeNull()
    expect(brandRamp('')).toBeNull()
  })
})
