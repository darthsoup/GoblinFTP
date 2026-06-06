import type { ApiEnvelope } from '~/types/api'
import { ApiError } from '~/types/api'

// Codes that mean the session/connection is gone — the UI switches to the
// blocking reconnect dialog instead of surfacing a raw error.
const SESSION_LOST_CODES = new Set(['ERR_SESSION_NOT_FOUND', 'ERR_UNAUTHORIZED', 'ERR_CSRF_INVALID', 'ERR_CONNECTION_LOST'])

export function useApi() {
  // Get auth store lazily (avoid circular deps at module load)
  function getCsrfToken(): string {
    const authStore = useAuthStore()
    return authStore.csrfToken
  }

  function raise(code: string, message: string): never {
    if (SESSION_LOST_CODES.has(code))
      useAuthStore().markSessionLost()
    throw new ApiError(code, message)
  }

  async function call<T>(method: 'GET' | 'POST' | 'PATCH' | 'DELETE', path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {}
    const upper = method.toUpperCase()
    if (upper !== 'GET' && upper !== 'HEAD') {
      const csrf = getCsrfToken()
      if (csrf)
        headers['X-CSRF-Token'] = csrf
    }

    try {
      const response = await $fetch<ApiEnvelope<T>>(path, {
        method,
        headers,
        body: body !== undefined ? body : undefined,
      })
      if (!response.success) {
        const err = response.errors?.[0]
        raise(err?.code ?? 'ERR_UNKNOWN', err?.message ?? 'Request failed')
      }
      return response.data as T
    }
    catch (e) {
      if (e instanceof ApiError)
        throw e
      // ofetch throws FetchError on non-2xx — the response body still carries
      // our envelope, so surface the real code + message instead of a raw
      // "[GET] ... 500" string.
      const envelope = (e as { data?: ApiEnvelope<unknown> }).data
      const err = envelope?.errors?.[0]
      if (err)
        raise(err.code, err.message)
      const msg = e instanceof Error ? e.message : 'Network error'
      throw new ApiError('ERR_NETWORK', msg)
    }
  }

  return {
    get: <T>(path: string) => call<T>('GET', path),
    post: <T>(path: string, body?: unknown) => call<T>('POST', path, body),
    patch: <T>(path: string, body?: unknown) => call<T>('PATCH', path, body),
    del: <T>(path: string, body?: unknown) => call<T>('DELETE', path, body),
  }
}
