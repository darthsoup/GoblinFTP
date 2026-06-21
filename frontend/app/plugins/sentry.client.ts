export default defineNuxtPlugin(async () => {
  const { public: pub } = useRuntimeConfig()
  const dsn = pub.sentryDsn as string | undefined
  if (!dsn)
    return

  const Sentry = await import('@sentry/nuxt')
  Sentry.init({
    dsn,
    tracesSampleRate: 1.0,
    beforeSend(event) {
      // Scrub PII: remove user context so usernames/hostnames are not captured.
      delete event.user
      return event
    },
  })
})
