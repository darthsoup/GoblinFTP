import { isAppLanguage } from '~/stores/settings'

// Resolves session + system vars before the auth middleware's first decision.
// Without it a restored cookie session bounces '/' → /login → '/' on a hard refresh.
export default defineNuxtPlugin(async (nuxtApp) => {
  const authStore = useAuthStore()
  const settingsStore = useSettingsStore()

  // Reactive getters registered before init(), so title/favicon update the moment
  // systemVars arrives and useHead still runs in the synchronous setup context.
  useHead(() => {
    const branding = authStore.systemVars?.branding
    const appName = branding?.appName || 'GoblinFTP'
    // ui.pageTitle is empty unless an admin sets it explicitly; then it wins
    // over the (branding) app name for the tab title only.
    const baseTitle = authStore.systemVars?.ui.pageTitle || appName
    // Server first, since tab titles truncate the tail. serverHost is host:port,
    // empty for a fresh SSO connect.
    const title = authStore.connected && authStore.serverHost
      ? `${authStore.serverHost} - ${baseTitle}`
      : baseTitle
    // unhead v3 types `rel` as a discriminated union, so each entry needs its own
    // concrete `rel` rather than a shared `rel: string`.
    const link: Array<
      { rel: 'icon', href: string, key: string } | { rel: 'stylesheet', href: string, key: string }
    > = []
    if (branding?.faviconUrl)
      link.push({ rel: 'icon', href: branding.faviconUrl, key: 'favicon' })
    // Per-tenant theme stylesheet: lands after the bundled main.css, so its
    // :root/.light/.dark overrides win by source order.
    if (branding?.themeCssUrl)
      link.push({ rel: 'stylesheet', href: branding.themeCssUrl, key: 'tenant-theme' })
    // <html lang> was never set, so a screen reader read all 13 locales with
    // English phonemes (WCAG 3.1.1). Reactive, so it follows the picker.
    const lang = (nuxtApp.$i18n as { locale?: { value: string } } | undefined)?.locale?.value
    return { title, link, htmlAttrs: { lang: lang || 'en' } }
  })

  await authStore.init()

  // Accent color (white-label): override the goblin scale at runtime. Skipped for
  // tenant themes, whose :root rules the inline --color-goblin-* would otherwise beat.
  const branding = authStore.systemVars?.branding
  if (!branding?.themeCssUrl) {
    applyBrandColor(branding?.primaryColor)
    // Only primary solid surfaces read --gftp-primary-text (not --ui-text-inverted),
    // so a light accent pairs with dark button text without breaking tooltips.
    if (branding?.primaryTextColor)
      document.documentElement.style.setProperty('--gftp-primary-text', branding.primaryTextColor)
  }

  // Precedence: explicit user choice > admin default > en. Applied here so a session
  // restored straight onto the workspace still gets the right locale.
  const i18n = nuxtApp.$i18n as {
    locale: { value: string }
    setLocale: (locale: string) => Promise<void>
  } | undefined
  const adminLang = authStore.systemVars?.language
  const preferred = settingsStore.language
    ?? (isAppLanguage(adminLang) ? adminLang : undefined)
  if (i18n && preferred && i18n.locale.value !== preferred)
    await i18n.setLocale(preferred)

  // Reopen editor tabs from a previous session (fire-and-forget; re-fetches
  // content, so only while the cookie session is still connected).
  if (authStore.connected)
    void useEditorStore().restore()
})
