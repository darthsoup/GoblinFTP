import type { ApiEnvelope } from '~/types/api'
import { ApiError, ERR_ABORTED, ERR_NETWORK, ERR_UNKNOWN } from '~/types/api'

// Codes that mean the session/connection is gone. The UI switches to the
// blocking reconnect dialog instead of surfacing a raw error.
const SESSION_LOST_CODES = new Set(['ERR_SESSION_NOT_FOUND', 'ERR_UNAUTHORIZED', 'ERR_CSRF_INVALID', 'ERR_CONNECTION_LOST'])

// Bytes handed to the socket, not bytes the server received: a body smaller
// than the kernel send buffer reports complete almost immediately.
export interface UploadProgress {
  loaded: number
  total: number
}

export function useApi() {
  // Resolved per call to avoid a circular dep at module load.
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
        raise(err?.code ?? ERR_UNKNOWN, err?.message ?? 'Request failed')
      }
      return response.data as T
    }
    catch (e) {
      throw await toApiError(e)
    }
  }

  // toApiError turns anything ofetch threw into an ApiError with a stable code,
  // marking the session lost first when the code says so. Callers throw the
  // result, which keeps the control flow visible to the type checker.
  async function toApiError(e: unknown): Promise<ApiError> {
    if (e instanceof ApiError)
      return e

    const err = await envelopeErrorFrom(e)
    if (err) {
      if (SESSION_LOST_CODES.has(err.code))
        useAuthStore().markSessionLost()
      return new ApiError(err.code, err.message)
    }

    // No envelope: a proxy error page, a dropped connection, a CORS failure.
    // ofetch's own text is "[GET] \"/api/files?path=%2F\": 502 Bad Gateway",
    // which must never reach the UI, so it is kept only as a fallback message.
    const status = (e as { status?: number, statusCode?: number }).status
      ?? (e as { statusCode?: number }).statusCode
      ?? 0
    const message = e instanceof Error ? e.message : 'Request failed'
    if (status === 0 || status === 502 || status === 503 || status === 504)
      return new ApiError(ERR_NETWORK, message)
    if (status === 413)
      return new ApiError('ERR_FILE_TOO_LARGE', message)
    return new ApiError(ERR_UNKNOWN, message)
  }

  // envelopeErrorFrom digs our {code,message} out of a FetchError body. With
  // responseType 'blob' the body arrives as a Blob, so it has to be read and
  // parsed rather than indexed, and a non-JSON proxy page must not throw.
  async function envelopeErrorFrom(e: unknown): Promise<{ code: string, message: string } | null> {
    const data = (e as { data?: unknown }).data
    if (!data)
      return null

    if (typeof Blob !== 'undefined' && data instanceof Blob) {
      try {
        const parsed = JSON.parse(await data.text()) as ApiEnvelope<unknown>
        return parsed.errors?.[0] ?? null
      }
      catch {
        return null
      }
    }
    return (data as ApiEnvelope<unknown>).errors?.[0] ?? null
  }

  // getBlob/postBlob share CSRF injection, the envelope unwrap and raise() with
  // call(). Before these, downloads used window.open and a bare $fetch.raw, so a
  // failure rendered raw JSON in a blank tab or bypassed markSessionLost().
  async function getBlob(path: string): Promise<Blob> {
    try {
      return await $fetch<Blob>(path, { responseType: 'blob' })
    }
    catch (e) {
      throw await toApiError(e)
    }
  }

  async function postBlob(path: string, body: unknown): Promise<Blob> {
    const headers: Record<string, string> = {}
    const csrf = getCsrfToken()
    if (csrf)
      headers['X-CSRF-Token'] = csrf
    try {
      return await $fetch<Blob>(path, {
        method: 'POST',
        headers,
        body: body as Record<string, unknown>,
        responseType: 'blob',
      })
    }
    catch (e) {
      throw await toApiError(e)
    }
  }

  // saveBlob hands the bytes to the browser as a download.
  function saveBlob(blob: Blob, filename: string): void {
    const url = URL.createObjectURL(blob)
    try {
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      a.remove()
    }
    finally {
      // Deferred: revoking synchronously can cancel the click-started download.
      setTimeout(() => URL.revokeObjectURL(url), 60_000)
    }
  }

  // XHR rather than $fetch: the fetch API emits no upload-progress events. It
  // lives here so CSRF, the envelope unwrap, ApiError and raise() stay shared.
  function postForm<T>(path: string, form: FormData, opts?: { onProgress?: (p: UploadProgress) => void, signal?: AbortSignal }): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      // Without this the request is unreachable once started: cancelling only
      // relabelled the row while the browser kept uploading the whole file.
      if (opts?.signal) {
        if (opts.signal.aborted) {
          reject(new ApiError(ERR_ABORTED, 'Request aborted'))
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
            raise(err?.code ?? ERR_UNKNOWN, err?.message ?? `Request failed (${xhr.status})`)
          }
          resolve(envelope!.data as T)
        }
        catch (e) {
          // raise() throws; reject rather than letting it escape the callback,
          // where nothing would catch it.
          reject(e)
        }
      }
      xhr.onerror = () => reject(new ApiError(ERR_NETWORK, 'Network error'))
      xhr.onabort = () => reject(new ApiError(ERR_ABORTED, 'Request aborted'))
      xhr.send(form)
    })
  }

  return {
    get: <T>(path: string) => call<T>('GET', path),
    post: <T>(path: string, body?: unknown) => call<T>('POST', path, body),
    patch: <T>(path: string, body?: unknown) => call<T>('PATCH', path, body),
    del: <T>(path: string, body?: unknown) => call<T>('DELETE', path, body),
    postForm,
    getBlob,
    postBlob,
    saveBlob,
  }
}
