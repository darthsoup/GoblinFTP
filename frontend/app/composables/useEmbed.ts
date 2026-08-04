// Chromeless embed state, shaped like useBranding().
//
// PRESENTATION ONLY. This hides UI; it never restricts what a user can do, and
// nothing on the server branches on it. Anything that must be *forbidden*
// inside a panel belongs in server-side config (editor.disabled,
// connection.lockHost, GFTP_DISABLE_LOGIN_FORM). Do not turn this into a
// security control.
//
// Detection is `window.self !== window.top` rather than a query param: being
// framed is a boot-time environment fact, whereas the SSO redirect and both
// auth redirects drop the query string, so a param would survive only the very
// first load.
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
