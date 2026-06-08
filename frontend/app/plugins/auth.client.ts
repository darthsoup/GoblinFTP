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
    return {
      title: branding?.appName || 'GoblinFTP',
      link: branding?.faviconUrl ? [{ rel: 'icon', href: branding.faviconUrl, key: 'favicon' }] : [],
    }
  })

  await authStore.init()

  // Accent color (white-label): override the goblin scale at runtime. No-op when
  // unset, so the default green stays.
  applyBrandColor(authStore.systemVars?.branding?.primaryColor)

  // Language precedence: explicit user choice > admin default > en. Applied
  // here (not on a page) so a restored session landing straight on the
  // workspace still gets the right locale.
  const i18n = nuxtApp.$i18n as {
    locale: { value: string }
    setLocale: (locale: string) => Promise<void>
  } | undefined
  const adminLang = authStore.systemVars?.language
  const preferred = settingsStore.language
    ?? (adminLang === 'en' || adminLang === 'de' ? adminLang : undefined)
  if (i18n && preferred && i18n.locale.value !== preferred)
    await i18n.setLocale(preferred)

  // Reopen editor tabs from a previous session (fire-and-forget; re-fetches
  // content, so only while the cookie session is still connected).
  if (authStore.connected)
    void useEditorStore().restore()
})
