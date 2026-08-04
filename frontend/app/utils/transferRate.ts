import type { SizeFormat } from '~/stores/settings'

// Moving-average throughput and ETA for in-flight transfers.
//
// Samples are taken on a fixed clock rather than on byte events. That is the
// load-bearing choice: it makes the estimator independent of transport
// granularity (identical math whether bytes arrive continuously or one chunk at
// a time), and it lets a stall report itself — a frozen byte count pushes equal
// values until the window shows no movement at all. Event-driven sampling
// cannot do that, because a stalled transfer produces no events to decay.

export const SAMPLE_INTERVAL_MS = 500
export const RATE_WINDOW_MS = 3000
// Below this span the slope is dominated by sampling jitter, so report "still
// measuring" instead of a wild number.
export const RATE_MIN_SPAN_MS = 1000
// An ETA this far out is extrapolation noise from a 3-second window, not
// information.
export const ETA_MAX_SECONDS = 86_400

export interface RateSample {
  at: number
  bytes: number
}

export function pushSample(samples: RateSample[], sample: RateSample, windowMs = RATE_WINDOW_MS): RateSample[] {
  const prev = samples[samples.length - 1]
  // A retry rewinds the byte count. The old window describes a different
  // attempt, so drop it rather than reporting a negative slope.
  const base = prev && sample.bytes < prev.bytes ? [] : samples
  const next = [...base, sample]
  const cutoff = sample.at - windowMs
  const firstInside = next.findIndex(s => s.at >= cutoff)
  // Keep the one sample just outside the cutoff so the span still covers the
  // full window rather than shrinking to the samples strictly inside it.
  return firstInside <= 0 ? next : next.slice(firstInside - 1)
}

// Bytes per second, or null when there is not yet enough signal to say.
// Zero is a real answer: it means the transfer is stalled.
export function rateFromSamples(samples: RateSample[], minSpanMs = RATE_MIN_SPAN_MS): number | null {
  if (samples.length < 2)
    return null
  const first = samples[0]!
  const last = samples[samples.length - 1]!
  const span = last.at - first.at
  if (span < minSpanMs)
    return null
  return Math.max(0, ((last.bytes - first.bytes) / span) * 1000)
}

export function etaSeconds(remainingBytes: number, bytesPerSecond: number | null): number | null {
  if (bytesPerSecond === null || bytesPerSecond <= 0)
    return null
  if (remainingBytes <= 0)
    return 0
  return remainingBytes / bytesPerSecond
}

// Zero-padded HH:MM:SS. Hours are uncapped so a genuinely long transfer reads
// 03:12:40 rather than wrapping.
export function formatEta(seconds: number | null): string {
  if (seconds === null || !Number.isFinite(seconds) || seconds < 0 || seconds > ETA_MAX_SECONDS)
    return '--:--:--'
  const total = Math.round(seconds)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

// Delegates to formatFileSize so the user's binary/decimal/bytes preference is
// honoured and no fourth unit ladder enters the codebase. "/s" stays
// untranslated for the same reason formatFileSize hardcodes KiB/MB.
export function formatRate(bytesPerSecond: number, format: SizeFormat, locale = 'en'): string {
  return `${formatFileSize(Math.max(0, Math.round(bytesPerSecond)), format, locale)}/s`
}
