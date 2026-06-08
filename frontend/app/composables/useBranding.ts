// White-label accessors with built-in fallbacks. Branding comes from systemVars
// (admin/env configured, instance-wide — no per-user override).
export function useBranding() {
  const authStore = useAuthStore()
  const { t } = useI18n()

  const branding = computed(() => authStore.systemVars?.branding)
  const appName = computed(() => branding.value?.appName || 'GoblinFTP')
  const logoUrl = computed(() => branding.value?.logoUrl ?? null)
  const tagline = computed(() => branding.value?.tagline || t('login.tagline'))
  const hideAttribution = computed(() => branding.value?.hideAttribution ?? false)

  return { appName, logoUrl, tagline, hideAttribution }
}
