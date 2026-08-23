// Polls GET /api/auth/status?ping=1, and re-checks when the tab regains focus.
// Skipped during transfers: a NOOP must not interleave with a running upload.
const CHECK_INTERVAL_MS = 30_000

export function useSessionChecker() {
  const authStore = useAuthStore()
  const uploadStore = useUploadStore()

  function tick() {
    if (!authStore.connected || authStore.sessionLost || uploadStore.hasActive)
      return
    authStore.checkSession()
  }

  // VueUse handles start/stop + cleanup on scope dispose.
  useIntervalFn(tick, CHECK_INTERVAL_MS)
  useEventListener(document, 'visibilitychange', () => {
    if (document.visibilityState === 'visible')
      tick()
  })
}
