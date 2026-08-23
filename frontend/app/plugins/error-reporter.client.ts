import type { FrontendErrorKind } from '~/utils/errorReport'

// Bare $fetch instead of useApi(): the endpoint is public (no CSRF/session), and the
// reporter must never trigger the session-lost machinery or loop on its own failures.
const MAX_REPORTS_PER_PAGE = 20

export default defineNuxtPlugin((nuxtApp) => {
  const seen = new Set<string>()
  let sent = 0

  // The SDK's own GlobalHandlers integration is disabled (plugins/sentry.client.ts),
  // so this is the only path from a browser error to Sentry. The returned event ID
  // tells the backend the error is already filed and its relay should stand down.
  async function captureInSentry(err: unknown): Promise<string | undefined> {
    const dsn = useRuntimeConfig().public.sentryDsn as string | undefined
    if (!dsn)
      return undefined
    try {
      const Sentry = await import('@sentry/nuxt')
      return Sentry.captureException(err) || undefined
    }
    catch {
      return undefined
    }
  }

  function report(kind: FrontendErrorKind, err: unknown, source?: string) {
    try {
      if (sent >= MAX_REPORTS_PER_PAGE)
        return

      const payload = buildErrorPayload(kind, err, window.location.pathname, source)
      const key = errorDedupeKey(payload)
      if (seen.has(key))
        return
      seen.add(key)
      sent++

      // Lazy store access: errors can fire before app state is ready. Sentry is
      // independent of the backend relay, so an admin who turned GFTP_LOG_FRONTEND
      // off but configured a browser DSN still gets their browser errors.
      const forwardToBackend = useAuthStore().systemVars?.frontendLogEnabled ?? false

      captureInSentry(err)
        .then((sentryEventId) => {
          if (!forwardToBackend)
            return
          return $fetch('/api/log/frontend', {
            method: 'POST',
            body: { ...payload, sentryEventId },
            retry: 0,
          })
        })
        .catch(() => {})
    }
    catch {
      // The reporter must never throw.
    }
  }

  nuxtApp.hook('vue:error', (err, _instance, info) => report('vue', err, info))

  window.addEventListener('error', (event) => {
    const source = event.filename ? `${event.filename}:${event.lineno}:${event.colno}` : undefined
    report('error', event.error ?? event.message, source)
  })
  window.addEventListener('unhandledrejection', (event) => {
    report('rejection', event.reason)
  })
})
