import type { DateFormat, SizeFormat } from '~/stores/settings'

const BINARY_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
const DECIMAL_UNITS = ['B', 'KB', 'MB', 'GB', 'TB']

// formatFileSize renders a byte count according to the user's size format:
// binary (KiB, 1024), decimal (KB, 1000), or exact bytes with separators.
export function formatFileSize(bytes: number, format: SizeFormat, locale = 'en'): string {
  if (format === 'bytes')
    return `${bytes.toLocaleString(locale)} B`
  const base = format === 'binary' ? 1024 : 1000
  const units = format === 'binary' ? BINARY_UNITS : DECIMAL_UNITS
  if (bytes < base)
    return `${bytes} ${units[0]}`
  let value = bytes
  let unit = 0
  while (value >= base && unit < units.length - 1) {
    value /= base
    unit++
  }
  return `${value.toFixed(1)} ${units[unit]}`
}

const RELATIVE_STEPS: Array<{ limit: number, divisor: number, unit: Intl.RelativeTimeFormatUnit }> = [
  { limit: 60, divisor: 1, unit: 'second' },
  { limit: 3600, divisor: 60, unit: 'minute' },
  { limit: 86400, divisor: 3600, unit: 'hour' },
  { limit: 7 * 86400, divisor: 86400, unit: 'day' },
]

// formatFileDate renders an ISO timestamp according to the user's date format:
// auto (compact, year only when it differs), absolute (full date + time), or
// relative ("2 hours ago", falling back to auto beyond a week).
export function formatFileDate(iso: string, format: DateFormat, locale = 'en'): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime()))
    return iso

  if (format === 'absolute') {
    return new Intl.DateTimeFormat(locale, {
      year: 'numeric',
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }).format(d)
  }

  if (format === 'relative') {
    const diffSeconds = (d.getTime() - Date.now()) / 1000
    const elapsed = Math.abs(diffSeconds)
    for (const step of RELATIVE_STEPS) {
      if (elapsed < step.limit) {
        const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
        return rtf.format(Math.round(diffSeconds / step.divisor), step.unit)
      }
    }
    // older than a week → compact absolute
  }

  const sameYear = d.getFullYear() === new Date().getFullYear()
  return new Intl.DateTimeFormat(locale, sameYear
    ? { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }
    : { year: 'numeric', month: 'short', day: '2-digit' }).format(d)
}
