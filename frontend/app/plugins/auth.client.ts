import { isAppLanguage } from '~/stores/settings'

// Resolves session + system vars (and applies the language preference) once,
// before the auth route middleware makes its first decision. This is what lets
// a cold load or hard refresh of any route land correctly — without it, a
// restored cookie session would bounce '/' → /login → '/' because `connected`
// defaults to false until init() runs.
export default defineNuxtPlugin(async (nuxtApp) => {
  const authStore = useAuthStore()
  const settingsStore = useSettingsStore()

  // Document title + favicon track the (white-label) branding. Registered with
  // reactive getters before init() so they update the moment systemVars arrives
  // (and so useHead runs inside the synchronous plugin-setup context).
  useHead(() => {
    const branding = authStore.systemVars?.branding
    const appName = branding?.appName || 'GoblinFTP'
    // Surface the connected server in the tab title (server first — tab titles
    // truncate the tail). serverHost is host:port; empty for a fresh SSO connect.
    const title = authStore.connected && authStore.serverHost
      ? `${authStore.serverHost} — ${appName}`
      : appName
    // unhead v3 types link `rel` as a discriminated union (one literal per
    // shape), so each entry needs its own concrete `rel` rather than a shared
    // `rel: string`.
    const link: Array<
      { rel: 'icon', href: string, key: string } | { rel: 'stylesheet', href: string, key: string }
    > = []
    if (branding?.faviconUrl)
      link.push({ rel: 'icon', href: branding.faviconUrl, key: 'favicon' })
    // Per-tenant theme stylesheet: lands after the bundled main.css, so its
    // :root/.light/.dark overrides win by source order.
    if (branding?.themeCssUrl)
      link.push({ rel: 'stylesheet', href: branding.themeCssUrl, key: 'tenant-theme' })
    return { title, link }
  })

  await authStore.init()

  // Accent color (white-label): override the goblin scale at runtime. No-op when
  // unset, so the default green stays. Skipped when a tenant theme stylesheet is
  // present — it drives --ui-primary directly, and applyBrandColor's inline
  // --color-goblin-* would otherwise beat the stylesheet's :root rules.
  const branding = authStore.systemVars?.branding
  if (!branding?.themeCssUrl) {
    applyBrandColor(branding?.primaryColor)
    // Selectable button/primary text color — drives --gftp-primary-text, which
    // only the primary solid surfaces read (not the shared --ui-text-inverted),
    // so a light accent (e.g. yellow) pairs with dark button text without
    // breaking tooltips. Skipped for tenant themes — their config.css owns it.
    if (branding?.primaryTextColor)
      document.documentElement.style.setProperty('--gftp-primary-text', branding.primaryTextColor)
  }

  // Language precedence: explicit user choice > admin default > en. Applied
  // here (not on a page) so a restored session landing straight on the
  // workspace still gets the right locale.
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
