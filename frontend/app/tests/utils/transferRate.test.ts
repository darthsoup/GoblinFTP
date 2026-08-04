import { describe, expect, it } from 'vitest'
import {
  ETA_MAX_SECONDS,
  etaSeconds,
  formatEta,
  formatRate,
  pushSample,
  rateFromSamples,
} from '~/utils/transferRate'

// Every case here is plain arithmetic on purpose: the whole point of keeping the
// estimator out of the component is that it needs no timers, mount or Pinia.

describe('pushSample', () => {
  it('keeps samples within the window', () => {
    let s = pushSample([], { at: 1000, bytes: 0 }, 3000)
    s = pushSample(s, { at: 1500, bytes: 100 }, 3000)
    s = pushSample(s, { at: 2000, bytes: 200 }, 3000)

    expect(s.map(x => x.at)).toEqual([1000, 1500, 2000])
  })

  // Trimming to samples strictly inside the cutoff would shrink the measured
  // span below the window and make the rate jumpier than intended.
  it('retains the sample just outside the cutoff so the span still covers the window', () => {
    let s: ReturnType<typeof pushSample> = []
    for (let at = 1000; at <= 6000; at += 500)
      s = pushSample(s, { at, bytes: at }, 3000)

    const span = s[s.length - 1]!.at - s[0]!.at
    expect(span).toBeGreaterThanOrEqual(3000)
    expect(s[0]!.at).toBe(2500)
  })

  it('discards the window when bytes go backwards (a retry restarted the item)', () => {
    let s = pushSample([], { at: 1000, bytes: 5000 })
    s = pushSample(s, { at: 1500, bytes: 9000 })
    s = pushSample(s, { at: 2000, bytes: 0 })

    expect(s).toEqual([{ at: 2000, bytes: 0 }])
    expect(rateFromSamples(s)).toBeNull()
  })
})

describe('rateFromSamples', () => {
  it('returns null with fewer than two samples', () => {
    expect(rateFromSamples([])).toBeNull()
    expect(rateFromSamples([{ at: 1000, bytes: 0 }])).toBeNull()
  })

  it('returns null while the span is below the minimum', () => {
    expect(rateFromSamples([{ at: 1000, bytes: 0 }, { at: 1500, bytes: 5_000_000 }])).toBeNull()
  })

  it('computes bytes per second across the window', () => {
    const rate = rateFromSamples([
      { at: 1000, bytes: 0 },
      { at: 3000, bytes: 2_000_000 },
    ])
    expect(rate).toBe(1_000_000)
  })

  // A frozen byte count is the stall signal. It must be 0, not null, so the UI
  // can say "Stalled" rather than silently rendering nothing.
  it('returns 0 for a frozen byte count', () => {
    expect(rateFromSamples([
      { at: 1000, bytes: 4096 },
      { at: 2500, bytes: 4096 },
      { at: 4000, bytes: 4096 },
    ])).toBe(0)
  })

  it('never reports a negative rate', () => {
    expect(rateFromSamples([{ at: 1000, bytes: 500 }, { at: 3000, bytes: 100 }])).toBe(0)
  })
})

describe('etaSeconds', () => {
  it('is null without a usable rate', () => {
    expect(etaSeconds(1000, null)).toBeNull()
    expect(etaSeconds(1000, 0)).toBeNull()
  })

  it('is 0 once nothing is left', () => {
    expect(etaSeconds(0, 1000)).toBe(0)
    expect(etaSeconds(-5, 1000)).toBe(0)
  })

  it('divides remaining bytes by the rate', () => {
    expect(etaSeconds(10_000, 1000)).toBe(10)
  })
})

describe('formatEta', () => {
  it('renders zero-padded HH:MM:SS', () => {
    expect(formatEta(0)).toBe('00:00:00')
    expect(formatEta(59)).toBe('00:00:59')
    expect(formatEta(60)).toBe('00:01:00')
    expect(formatEta(3600)).toBe('01:00:00')
    expect(formatEta(11_560)).toBe('03:12:40')
  })

  // Hours are not clamped to 24 in the formatting itself; what bounds the
  // output is ETA_MAX_SECONDS, which happens to be 24h.
  it('renders double-digit hours up to the cap', () => {
    expect(formatEta(20 * 3600)).toBe('20:00:00')
    expect(formatEta(ETA_MAX_SECONDS)).toBe('24:00:00')
  })

  it('renders placeholders for anything it cannot honestly claim', () => {
    expect(formatEta(null)).toBe('--:--:--')
    expect(formatEta(Number.POSITIVE_INFINITY)).toBe('--:--:--')
    expect(formatEta(Number.NaN)).toBe('--:--:--')
    expect(formatEta(-1)).toBe('--:--:--')
    // A multi-day estimate from a 3-second window is noise, not information.
    expect(formatEta(ETA_MAX_SECONDS + 1)).toBe('--:--:--')
  })
})

describe('formatRate', () => {
  it('honours the size format and appends /s', () => {
    expect(formatRate(1_048_576, 'binary')).toBe('1.0 MiB/s')
    expect(formatRate(1_000_000, 'decimal')).toBe('1.0 MB/s')
  })

  it('rounds sub-unit rates rather than leaking a fraction', () => {
    expect(formatRate(512.7, 'binary')).toBe('513 B/s')
  })
})
