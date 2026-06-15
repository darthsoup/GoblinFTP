// Last path segment, for compact failure labels.
export function basename(p: string): string {
  return p.split('/').filter(Boolean).pop() ?? p
}

// Pure formatter (no i18n) so it is unit-testable: one "label — reason" line per
// failure, capped, with a caller-supplied "+N more" tail.
export function formatFailureLines(
  items: { label: string, reason: string }[],
  cap: number,
  more: (n: number) => string,
): string {
  const lines = items.slice(0, cap).map(i => `${i.label} — ${i.reason}`)
  if (items.length > cap)
    lines.push(more(items.length - cap))
  return lines.join('\n')
}

// Localizes backend error codes and formats per-item failure lists for toasts.
// `localizeError` maps a stable code (e.g. ERR_DIR_NOT_EMPTY) to an `errorCode.*`
// i18n string, falling back to the server-provided friendly message for codes the
// SPA hasn't localized yet.
export function useErrorMessage() {
  const { t, te } = useI18n()

  function localizeError(code: string, fallback: string): string {
    const key = `errorCode.${code}`
    return code && te(key) ? t(key) : fallback
  }

  function formatFailures(items: { label: string, reason: string }[], cap = 5): string {
    return formatFailureLines(items, cap, n => t('toast.andMore', { n }))
  }

  return { localizeError, formatFailures }
}
