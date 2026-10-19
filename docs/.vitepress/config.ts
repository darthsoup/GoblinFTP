import { defineConfig } from 'vitepress'

export default defineConfig({
  lang: 'en-US',
  title: 'GoblinFTP',
  description: 'Upload, download, and edit files on your FTP, FTPS, or SFTP server from your browser.',
  base: '/GoblinFTP/',
  cleanUrls: false,
  // Installation examples point to the reader's own running instance.
  ignoreDeadLinks: 'localhostLinks',
  markdown: { languageAlias: { caddy: 'text', promql: 'text' } },
  lastUpdated: true,
  sitemap: { hostname: 'https://darthsoup.github.io/GoblinFTP/' },
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/GoblinFTP/server.svg' }],
    ['meta', { name: 'theme-color', content: '#006e16' }],
  ],
  themeConfig: {
    logo: { light: '/server.svg', dark: '/server-dark.svg', alt: 'GoblinFTP server' },
    nav: [
      { text: 'Get started', link: '/installation', activeMatch: '/(installation|system-requirements)' },
      { text: 'Configuration', link: '/configuration' },
      { text: 'Contributing', link: '/development', activeMatch: '/(development|i18n)' },
    ],
    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Installation', link: '/installation' },
          { text: 'System Requirements', link: '/system-requirements' },
        ],
      },
      {
        text: 'Configuration',
        items: [
          { text: 'Configuration Reference', link: '/configuration' },
          { text: 'Branding & themes', link: '/theming' },
        ],
      },
      {
        text: 'Integrations',
        items: [
          { text: 'Login links (SSO)', link: '/sso' },
          { text: 'Embed in your app', link: '/embedding' },
        ],
      },
      {
        text: 'Monitoring & storage',
        items: [
          { text: 'Logging', link: '/logging' },
          { text: 'Metrics', link: '/metrics' },
          { text: 'Error tracking', link: '/sentry' },
          { text: 'S3 upload storage', link: '/s3-staging' },
        ],
      },
      {
        text: 'Contributing',
        items: [
          { text: 'Development', link: '/development' },
          { text: 'Translations', link: '/i18n' },
        ],
      },
    ],
    search: { provider: 'local' },
    outline: [2, 3],
    editLink: {
      pattern: 'https://github.com/darthsoup/goblinftp/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
    socialLinks: [{ icon: 'github', link: 'https://github.com/darthsoup/goblinftp' }],
    footer: { message: 'Released under the MIT License.' },
  },
})
