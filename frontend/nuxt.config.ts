import process from 'node:process'
import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  ssr: false,

  // Dev parity with docker/Caddyfile, which owns this header in production. Nitro
  // serves the dev document, so it must be a route rule: vite.server.headers misses it.
  routeRules: {
    '/**': {
      headers: {
        'Content-Security-Policy': `frame-ancestors ${process.env.GFTP_FRAME_ANCESTORS || '\'none\''}`,
      },
    },
  },

  app: {
    head: {
      viewport: 'width=device-width, initial-scale=1, viewport-fit=cover',
    },
  },

  components: [
    { path: '~/components', pathPrefix: false },
  ],

  modules: [
    // First, so its auto-imports yield to Nuxt's own on any name collision
    // (notably useColorMode, owned by @nuxt/ui's color-mode integration).
    '@vueuse/nuxt',
    '@nuxt/eslint',
    '@nuxt/ui',
    '@pinia/nuxt',
    '@nuxtjs/i18n',
  ],

  // All four are baked into the static SPA at `nuxt generate`, unlike the GFTP_*
  // vars the backend reads at process start.
  runtimeConfig: {
    public: {
      sentryDsn: process.env.NUXT_PUBLIC_SENTRY_DSN ?? '',
      sentryEnvironment: process.env.NUXT_PUBLIC_SENTRY_ENVIRONMENT ?? '',
      sentryRelease: process.env.NUXT_PUBLIC_SENTRY_RELEASE ?? process.env.VERSION ?? '',
      sentryTracesSampleRate: process.env.NUXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE ?? '0',
    },
  },

  // Hidden client source maps: emitted for upload to Sentry but not referenced
  // from the bundles, so browsers never fetch them. See docs/sentry.md.
  sourcemap: { client: 'hidden' },

  // Default to the system preference (settings modal: Automatic); when the
  // system preference is unknown, the brand default is dark.
  colorMode: {
    preference: 'system',
    fallback: 'dark',
  },

  i18n: {
    locales: [
      { code: 'en', file: 'en.json' },
      { code: 'de', file: 'de.json' },
      { code: 'cs', file: 'cs.json' },
      { code: 'da', file: 'da.json' },
      { code: 'es', file: 'es.json' },
      { code: 'fi', file: 'fi.json' },
      { code: 'fr', file: 'fr.json' },
      { code: 'it', file: 'it.json' },
      { code: 'nb-NO', file: 'nb-NO.json' },
      { code: 'nl', file: 'nl.json' },
      { code: 'pt', file: 'pt.json' },
      { code: 'sk', file: 'sk.json' },
      { code: 'sv', file: 'sv.json' },
    ],
    defaultLocale: 'en',
    strategy: 'no_prefix',
    // Language is applied on boot: user choice (gftp_settings localStorage)
    // > admin default (GFTP_LANGUAGE) > en. No browser detection.
    detectBrowserLanguage: false,
  },

  devtools: { enabled: true },

  typescript: {
    strict: true,
  },

  compatibilityDate: '2026-05-01',

  css: ['~/assets/css/main.css'],

  vite: {
    plugins: [
      tailwindcss(),
    ],
    build: {
      // The ~505KB entry (174KB gz) is the irreducible @nuxt/ui + Vue core; Sentry and
      // CodeMirror already split out, and manualChunks on @nuxt/ui only made it worse.
      chunkSizeWarningLimit: 600,
    },
    server: {
      proxy: {
        '/api': {
          target: process.env.GFTP_DEV_PROXY ?? 'http://localhost:8080',
          changeOrigin: true,
        },
        // Per-tenant theme assets are served by the Go backend (Caddy does this in prod).
        '/themes': {
          target: process.env.GFTP_DEV_PROXY ?? 'http://localhost:8080',
          changeOrigin: true,
        },
        // Forward only `GET /?sso=<token>` so the one-time-link flow stays same-origin
        // on :3000; every other `/` request goes back to Vite. Prod: docker/Caddyfile @sso.
        '/': {
          target: process.env.GFTP_DEV_PROXY ?? 'http://localhost:8080',
          changeOrigin: true,
          bypass(req) {
            const isSsoEntry = req.method === 'GET' && /[?&]sso=/.test(req.url ?? '')
            // undefined → proxy to backend; req.url → serve locally (SPA).
            return isSsoEntry ? undefined : req.url
          },
        },
      },
    },
  },
})
