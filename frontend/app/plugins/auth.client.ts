// Resolves session + system vars (and applies the language preference) once,
// before the auth route middleware makes its first decision. This is what lets
// a cold load or hard refresh of any route land correctly — without it, a
// restored cookie session would bounce '/' → /login → '/' because `connected`
// defaults to false until init() runs.
export default defineNuxtPlugin(async (nuxtApp) => {
  const authStore = useAuthStore()
  const settingsStore = useSettingsStore()

  await authStore.init()

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
})
