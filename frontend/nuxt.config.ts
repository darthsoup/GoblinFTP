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

  i18n: {
    locales: [
      { code: 'en', file: 'en.json' },
      { code: 'de', file: 'de.json' },
    ],
    defaultLocale: 'en',
    strategy: 'no_prefix',
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
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
      },
    },
  },
})
