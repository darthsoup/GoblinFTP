import { scrubUrl } from '~/utils/sentryScrub'

// Runs before error-reporter.client.ts (alphabetical is not guaranteed, so the
// reporter looks the SDK up lazily rather than assuming this has initialized).
export default defineNuxtPlugin(async () => {
  const { public: pub } = useRuntimeConfig()
  const dsn = pub.sentryDsn as string | undefined
  if (!dsn)
    return

  const Sentry = await import('@sentry/nuxt')
  Sentry.init({
    dsn,
    environment: (pub.sentryEnvironment as string) || undefined,
    // Without a release, a browser stack trace cannot be tied to a deployed
    // version, and source maps can never be matched to it either.
    release: (pub.sentryRelease as string) || undefined,
    tracesSampleRate: Number(pub.sentryTracesSampleRate ?? 0),
    sendDefaultPii: false,
    // GlobalHandlers dropped on purpose: error-reporter.client.ts is the single
    // funnel for browser errors, so the same throw cannot be filed twice (once by
    // the SDK, once by the reporter) and the reporter's dedupe and per-page cap
    // bound the Sentry quota too.
    integrations: defaults => defaults.filter(i => i.name !== 'GlobalHandlers'),
    beforeBreadcrumb(breadcrumb) {
      // fetch/xhr breadcrumbs record the URL verbatim, which is how the 15-minute
      // download token and remote file paths would otherwise reach Sentry.
      if (typeof breadcrumb.data?.url === 'string')
        breadcrumb.data.url = scrubUrl(breadcrumb.data.url)
      if (breadcrumb.category === 'navigation') {
        breadcrumb.data = {
          ...breadcrumb.data,
          from: scrubUrl(breadcrumb.data?.from),
          to: scrubUrl(breadcrumb.data?.to),
        }
      }
      return breadcrumb
    },
    beforeSend(event) {
      // Scrub PII: remove user context so usernames/hostnames are not captured.
      delete event.user
      if (event.request?.url)
        event.request.url = scrubUrl(event.request.url)
      if (event.request?.query_string)
        event.request.query_string = '[Filtered]'
      return event
    },
  })
})
