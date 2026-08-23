// Runtime accent-color theming for white-labeling: one brand hex becomes an 11-stop
// scale (anchored at 500) overriding the `goblin` vars, so --ui-primary stays correct.

export type RampKey = '50' | '100' | '200' | '300' | '400' | '500' | '600' | '700' | '800' | '900' | '950'
export type Ramp = Record<RampKey, string>

// Mix amount per stop: >0 lightens (toward white), <0 darkens (toward black),
// 0 is the supplied base color.
const STOPS: Array<[RampKey, number]> = [
  ['50', 0.92],
  ['100', 0.84],
  ['200', 0.68],
  ['300', 0.48],
  ['400', 0.24],
  ['500', 0],
  ['600', -0.12],
  ['700', -0.28],
  ['800', -0.44],
  ['900', -0.6],
  ['950', -0.74],
]

function parseHex(hex: string): [number, number, number] | null {
  let h = hex.trim().replace(/^#/, '')
  if (h.length === 3)
    h = h.split('').map(c => c + c).join('')
  if (!/^[0-9a-f]{6}$/i.test(h))
    return null
  return [Number.parseInt(h.slice(0, 2), 16), Number.parseInt(h.slice(2, 4), 16), Number.parseInt(h.slice(4, 6), 16)]
}

function channelHex(n: number): string {
  return Math.round(Math.max(0, Math.min(255, n))).toString(16).padStart(2, '0')
}

function mix(c: number, amount: number): number {
  return amount >= 0 ? c + (255 - c) * amount : c * (1 + amount)
}

// Build an 11-stop scale from a base hex, or null if the hex is malformed.
export function brandRamp(hex: string): Ramp | null {
  const rgb = parseHex(hex)
  if (!rgb)
    return null
  const [r, g, b] = rgb
  const ramp = {} as Ramp
  for (const [key, amount] of STOPS)
    ramp[key] = `#${channelHex(mix(r, amount))}${channelHex(mix(g, amount))}${channelHex(mix(b, amount))}`
  return ramp
}

// Override the goblin scale on :root with a ramp derived from `hex`. No-op when
// hex is empty/invalid (keeps the built-in green).
export function applyBrandColor(hex: string | null | undefined): void {
  if (!hex)
    return
  const ramp = brandRamp(hex)
  if (!ramp)
    return
  const root = document.documentElement
  for (const key of Object.keys(ramp) as RampKey[])
    root.style.setProperty(`--color-goblin-${key}`, ramp[key])
}
