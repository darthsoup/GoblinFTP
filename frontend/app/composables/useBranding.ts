// White-label accessors with built-in fallbacks. Branding comes from systemVars
// (admin/env configured, instance-wide - no per-user override).
export function useBranding() {
  const authStore = useAuthStore()
  const colorMode = useColorMode()

  const branding = computed(() => authStore.systemVars?.branding)
  const appName = computed(() => branding.value?.appName || 'GoblinFTP')
  // Prefer the dark-mode logo when in dark mode and one is provided - a
  // light-mode wordmark (dark ink) is otherwise illegible on the dark canvas.
  const logoUrl = computed(() => {
    const b = branding.value
    if (colorMode.value === 'dark' && b?.logoDarkUrl)
      return b.logoDarkUrl
    return b?.logoUrl ?? null
  })
  const hideAttribution = computed(() => branding.value?.hideAttribution ?? false)

  return { appName, logoUrl, hideAttribution }
}
