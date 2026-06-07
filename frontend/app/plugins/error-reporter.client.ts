import type { FrontendErrorKind } from '~/utils/errorReport'

// Forwards uncaught browser errors to POST /api/log/frontend so they show up
// in the server's central log. Invisible to the user; coexists with Sentry.
//
// Deliberately uses bare $fetch instead of useApi(): the endpoint is public
// (no CSRF/session), and the reporter must never trigger the session-lost
// machinery or loop on its own failures.
const MAX_REPORTS_PER_PAGE = 20

export default defineNuxtPlugin((nuxtApp) => {
  const seen = new Set<string>()
  let sent = 0

  function report(kind: FrontendErrorKind, err: unknown, source?: string) {
    try {
      // Lazy store access: errors can fire before app state is ready.
      const auth = useAuthStore()
      if (!auth.systemVars?.frontendLogEnabled)
        return
      if (sent >= MAX_REPORTS_PER_PAGE)
        return

      const payload = buildErrorPayload(kind, err, window.location.pathname, source)
      const key = errorDedupeKey(payload)
      if (seen.has(key))
        return
      seen.add(key)
      sent++

      $fetch('/api/log/frontend', { method: 'POST', body: payload, retry: 0 }).catch(() => {})
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
