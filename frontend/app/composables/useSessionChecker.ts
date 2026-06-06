// Periodically verifies the session + FTP/SFTP connection are really alive
// (GET /api/auth/status?ping=1). Also re-checks when the tab regains focus.
// Polls are skipped during active transfers: a NOOP must not interleave with
// a running upload on the same control connection.
const CHECK_INTERVAL_MS = 30_000

export function useSessionChecker() {
  const authStore = useAuthStore()
  const uploadStore = useUploadStore()

  let timer: ReturnType<typeof setInterval> | null = null

  function tick() {
    if (!authStore.connected || authStore.sessionLost || uploadStore.hasActive)
      return
    authStore.checkSession()
  }

  function onVisibilityChange() {
    if (document.visibilityState === 'visible')
      tick()
  }

  onMounted(() => {
    timer = setInterval(tick, CHECK_INTERVAL_MS)
    document.addEventListener('visibilitychange', onVisibilityChange)
  })

  onUnmounted(() => {
    if (timer)
      clearInterval(timer)
    document.removeEventListener('visibilitychange', onVisibilityChange)
  })
}
