import process from 'node:process'
import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  ssr: false,

  components: [
    { path: '~/components', pathPrefix: false },
  ],

  modules: [
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
      },
    },
  },
})
