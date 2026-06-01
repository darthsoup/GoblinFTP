export default defineNuxtConfig({
  ssr: false,

  components: [
    { path: '~/components', pathPrefix: false },
  ],

  modules: [
    '@nuxt/ui',
    '@pinia/nuxt',
    '@nuxtjs/i18n',
  ],

  i18n: {
    locales: [
      { code: 'en', file: 'en.json' },
      { code: 'de', file: 'de.json' },
    ],
    defaultLocale: 'en',
    strategy: 'no_prefix',
  },

  devtools: { enabled: false },

  typescript: {
    strict: true,
  },

  compatibilityDate: '2026-05-01',

  vite: {
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
