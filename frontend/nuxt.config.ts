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
