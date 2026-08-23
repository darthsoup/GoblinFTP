import type { ApiEnvelope } from '~/types/api'
import { ApiError } from '~/types/api'

// Codes that mean the session/connection is gone - the UI switches to the
// blocking reconnect dialog instead of surfacing a raw error.
const SESSION_LOST_CODES = new Set(['ERR_SESSION_NOT_FOUND', 'ERR_UNAUTHORIZED', 'ERR_CSRF_INVALID', 'ERR_CONNECTION_LOST'])

// Bytes handed to the socket, which is not the same as bytes the server has
// received: a body smaller than the kernel send buffer reports complete almost
// immediately. Good enough for a rate on the transfers long enough to need one.
export interface UploadProgress {
  loaded: number
  total: number
}

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
      // ofetch throws FetchError on non-2xx - the response body still carries
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

  // XHR rather than $fetch: the fetch API emits no upload-progress events, so a
  // file small enough to fit in one request would report nothing at all. It
  // lives here, not in the upload store, so CSRF injection, the envelope
  // unwrap, ApiError and the session-lost side effect in raise() are shared
  // with the other methods instead of duplicated.
  function postForm<T>(path: string, form: FormData, opts?: { onProgress?: (p: UploadProgress) => void, signal?: AbortSignal }): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      // Without this the request is unreachable once started: cancelling only
      // relabelled the row while the browser kept uploading the whole file.
      if (opts?.signal) {
        if (opts.signal.aborted) {
          reject(new ApiError('ERR_ABORTED', 'Request aborted'))
          return
        }
        opts.signal.addEventListener('abort', () => xhr.abort(), { once: true })
      }
      xhr.open('POST', path)
      const csrf = getCsrfToken()
      if (csrf)
        xhr.setRequestHeader('X-CSRF-Token', csrf)
      xhr.responseType = 'json'

      if (opts?.onProgress) {
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable)
            opts.onProgress!({ loaded: e.loaded, total: e.total })
        }
      }

      xhr.onload = () => {
        const envelope = xhr.response as ApiEnvelope<T> | null
        try {
          if (!envelope || envelope.success === false || xhr.status >= 400) {
            const err = envelope?.errors?.[0]
            raise(err?.code ?? 'ERR_UNKNOWN', err?.message ?? `Request failed (${xhr.status})`)
          }
          resolve(envelope!.data as T)
        }
        catch (e) {
          // raise() throws; reject rather than letting it escape the callback,
          // where nothing would catch it.
          reject(e)
        }
      }
      xhr.onerror = () => reject(new ApiError('ERR_NETWORK', 'Network error'))
      xhr.onabort = () => reject(new ApiError('ERR_ABORTED', 'Request aborted'))
      xhr.send(form)
    })
  }

  return {
    get: <T>(path: string) => call<T>('GET', path),
    post: <T>(path: string, body?: unknown) => call<T>('POST', path, body),
    patch: <T>(path: string, body?: unknown) => call<T>('PATCH', path, body),
    del: <T>(path: string, body?: unknown) => call<T>('DELETE', path, body),
    postForm,
  }
}
