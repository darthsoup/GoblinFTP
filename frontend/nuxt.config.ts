import process from 'node:process'
import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  ssr: false,

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

  runtimeConfig: {
    public: {
      sentryDsn: process.env.NUXT_PUBLIC_SENTRY_DSN ?? '',
    },
  },

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
    // > admin default (settings.json) > en. No browser detection.
    detectBrowserLanguage: false,
  },

  devtools: { enabled: true },

  typescript: {
    strict: true,
  },

  compatibilityDate: '2026-05-01',

  css: ['./app/assets/css/main.css'],

  vite: {
    plugins: [
      tailwindcss(),
    ],
    build: {
      // The entry settles at ~505KB (174KB gz) — the irreducible @nuxt/ui + Vue
      // core. Sentry and the CodeMirror grammars are already split into their own
      // chunks; consolidating @nuxt/ui (~767KB) via manualChunks only made it
      // worse, so we raise the warning bar rather than chase a counterproductive split.
      chunkSizeWarningLimit: 600,
    },
    server: {
      proxy: {
        '/api': {
          // override with GFTP_DEV_PROXY to point at a backend on another port
          target: process.env.GFTP_DEV_PROXY ?? 'http://localhost:8080',
          changeOrigin: true,
        },
        // SSO entry point: forward only `GET /?sso=<token>` to the backend so
        // the one-time-link flow (decrypt → set session cookie → redirect to
        // /?) works in dev and stays same-origin on :3000. Every other request
        // to `/` (the SPA, assets, HMR) is bypassed back to the Vite dev server.
        // In prod, Caddy does this routing instead (docker/Caddyfile @sso).
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
