// PRESENTATION ONLY: this hides UI, it never restricts what a user can do.
// Anything that must be forbidden in a panel belongs in server-side config.

// Detection is `window.self !== window.top`, not a query param: the SSO and auth
// redirects drop the query string, so a param would survive only the first load.
export function useEmbed() {
  const authStore = useAuthStore()

  const framed = computed(() => {
    if (import.meta.server || typeof window === 'undefined')
      return false
    try {
      return window.self !== window.top
    }
    catch {
      // Reading window.top cross-origin throws, which itself proves we are
      // inside a frame on another origin.
      return true
    }
  })

  const embedded = computed(() => {
    const mode = authStore.systemVars?.embed?.chromeless ?? 'auto'
    if (mode === 'on')
      return true
    if (mode === 'off')
      return false
    return framed.value
  })

  return { embedded, framed }
}
