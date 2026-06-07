import type { AuthStatus, ConnectData, ConnectRequest, SystemVars } from '~/types/api'
import { defineStore } from 'pinia'
import { ApiError } from '~/types/api'

export const useAuthStore = defineStore('auth', () => {
  const csrfToken = ref('')
  const connected = ref(false)
  const ssoAutoConnect = ref(false)
  const serverHost = ref('') // set on manual connect; restored from /status on reload (host:port). Empty for a fresh SSO connect.
  const initialDirectory = ref('/')
  const capabilities = ref<{ disableChmod: boolean }>({ disableChmod: false })
  const systemVars = ref<SystemVars | null>(null)
  const error = ref<string | null>(null)
  const loading = ref(false)
  const sessionLost = ref(false)
  let disconnecting = false

  // Called on app mount — fetches system vars + auth status using $fetch directly
  // (no CSRF needed for these GET requests, and avoids circular dep with useApi)
  async function init() {
    try {
      const svRes = await $fetch<{ success: boolean, data?: SystemVars }>('/api/system/vars')
      if (svRes.success && svRes.data)
        systemVars.value = svRes.data
    }
    catch {}

    try {
      const statusRes = await $fetch<{ success: boolean, data?: AuthStatus }>('/api/auth/status')
      if (statusRes.success && statusRes.data) {
        const data = statusRes.data
        connected.value = data.connected
        ssoAutoConnect.value = data.ssoAutoConnect
        if (data.csrfToken)
          csrfToken.value = data.csrfToken
        // Restore the connection context after a page reload (in-memory state
        // is otherwise lost while the cookie session keeps us connected).
        if (data.connected) {
          if (data.host)
            serverHost.value = data.host
          if (data.initialDirectory)
            initialDirectory.value = data.initialDirectory
          if (data.capabilities)
            capabilities.value = data.capabilities
        }
      }
    }
    catch {}
  }

  // SSO auto-connect: called when ssoAutoConnect=true after init()
  async function ssoConnect() {
    loading.value = true
    error.value = null
    try {
      const api = useApi()
      const data = await api.post<ConnectData>('/api/auth/sso-connect')
      csrfToken.value = data.csrfToken
      connected.value = true
      ssoAutoConnect.value = false
      initialDirectory.value = data.initialDirectory
      capabilities.value = data.capabilities
    }
    catch (e) {
      error.value = e instanceof ApiError ? e.message : 'SSO connect failed'
      // Don't leave the client poised to auto-retry a failed finalization on
      // the next /login load — fall back to the manual form.
      ssoAutoConnect.value = false
    }
    finally {
      loading.value = false
    }
  }

  // Manual connect from login form
  async function connect(req: ConnectRequest) {
    loading.value = true
    error.value = null
    try {
      // POST /api/auth/connect is public (no CSRF), use $fetch directly
      const res = await $fetch<{ success: boolean, data?: ConnectData, errors?: Array<{ code: string, message: string }> }>(
        '/api/auth/connect',
        { method: 'POST', body: req },
      )
      if (!res.success) {
        const err = res.errors?.[0]
        throw new ApiError(err?.code ?? 'ERR_UNKNOWN', err?.message ?? 'Login failed')
      }
      const data = res.data!
      csrfToken.value = data.csrfToken
      connected.value = true
      serverHost.value = req.host
      initialDirectory.value = data.initialDirectory
      capabilities.value = data.capabilities
    }
    catch (e) {
      error.value = e instanceof ApiError ? e.message : 'Connection failed'
      throw e
    }
    finally {
      loading.value = false
    }
  }

  async function disconnect() {
    const api = useApi()
    disconnecting = true
    try {
      await api.post('/api/auth/disconnect')
    }
    catch {}
    finally {
      disconnecting = false
    }
    // Reset state regardless of API result
    csrfToken.value = ''
    connected.value = false
    ssoAutoConnect.value = false
    serverHost.value = ''
    initialDirectory.value = '/'
    error.value = null
  }

  // ── Session liveness ──────────────────────────────────────────────────────

  // Flags the session as lost — the UI shows a blocking reconnect dialog.
  // Ignored while a user-initiated disconnect is in flight.
  function markSessionLost() {
    if (!connected.value || sessionLost.value || disconnecting)
      return
    sessionLost.value = true
  }

  // Asks the backend to verify the FTP/SFTP connection with a real round
  // trip (?ping=1). Network errors are ignored — a flaky poll must not kill
  // the session; only a definitive connected=false does.
  async function checkSession() {
    if (!connected.value || sessionLost.value)
      return
    try {
      const res = await $fetch<{ success: boolean, data?: AuthStatus }>('/api/auth/status?ping=1')
      if (res.success && res.data && !res.data.connected)
        markSessionLost()
    }
    catch {}
  }

  // Acknowledge the lost session (reconnect button): back to the login form.
  function acknowledgeSessionLost() {
    sessionLost.value = false
    csrfToken.value = ''
    connected.value = false
    ssoAutoConnect.value = false
    serverHost.value = ''
    initialDirectory.value = '/'
    error.value = null
  }

  const allowedTypes = computed(() =>
    systemVars.value?.connection.allowedTypes ?? ['ftp', 'sftp'],
  )

  return {
    csrfToken,
    connected,
    ssoAutoConnect,
    serverHost,
    initialDirectory,
    capabilities,
    systemVars,
    error,
    loading,
    sessionLost,
    allowedTypes,
    init,
    ssoConnect,
    connect,
    disconnect,
    markSessionLost,
    checkSession,
    acknowledgeSessionLost,
  }
})
