import type { ApiEnvelope } from '~/types/api'
import { ApiError, ERR_ABORTED, ERR_NETWORK, ERR_UNKNOWN } from '~/types/api'
import { assertConnection, connectionEpoch, connectionRequest } from '~/utils/connection'

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
  const epoch = connectionEpoch()
  // Resolved per call to avoid a circular dep at module load.
  function getCsrfToken(path = ''): string {
    const authStore = useAuthStore()
    return path.includes('sso-') ? (authStore.ssoPending?.csrfToken ?? '') : authStore.csrfToken
  }

  function raise(code: string, message: string, sessionScope = true): never {
    if (sessionScope && SESSION_LOST_CODES.has(code))
      useAuthStore().markSessionLost()
    throw new ApiError(code, message)
  }

  async function call<T>(method: 'GET' | 'POST' | 'PATCH' | 'DELETE', path: string, body?: unknown): Promise<T> {
    assertConnection(epoch)
    const request = connectionRequest()
    const headers: Record<string, string> = {}
    const upper = method.toUpperCase()
    if (upper !== 'GET' && upper !== 'HEAD') {
      const csrf = getCsrfToken(path)
      if (csrf)
        headers['X-CSRF-Token'] = csrf
    }

    try {
      const response = await $fetch<ApiEnvelope<T>>(path, {
        method,
        signal: request.controller.signal,
        headers,
        body: body !== undefined ? body : undefined,
      })
      assertConnection(epoch)
      if (!response.success) {
        const err = response.errors?.[0]
        raise(err?.code ?? ERR_UNKNOWN, err?.message ?? 'Request failed', !path.includes('/auth/sso-'))
      }
      return response.data as T
    }
    catch (e) {
      assertConnection(epoch)
      throw await toApiError(e, !path.includes('/auth/sso-'))
    }
    finally { request.release() }
  }

  // toApiError turns anything ofetch threw into an ApiError with a stable code,
  // marking the session lost first when the code says so. Callers throw the
  // result, which keeps the control flow visible to the type checker.
  async function toApiError(e: unknown, sessionScope = true): Promise<ApiError> {
    if (e instanceof ApiError)
      return e

    const err = await envelopeErrorFrom(e)
    assertConnection(epoch)
    if (err) {
      if (sessionScope && SESSION_LOST_CODES.has(err.code))
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
  async function getBlob(path: string, maxBytes?: number): Promise<Blob> {
    assertConnection(epoch)
    const request = connectionRequest()
    try {
      const response = await $fetch.raw<ReadableStream<Uint8Array>>(path, { responseType: 'stream', signal: request.controller.signal })
      const length = Number(response.headers.get('Content-Length'))
      if (maxBytes !== undefined && length > maxBytes) {
        request.controller.abort()
        throw new ApiError('ERR_FILE_TOO_LARGE', 'Preview exceeds the maximum size')
      }
      const reader = response._data?.getReader()
      if (!reader)
        throw new ApiError(ERR_NETWORK, 'Empty response')
      const chunks: ArrayBuffer[] = []
      let received = 0
      try {
        for (;;) {
          const { done, value } = await reader.read()
          assertConnection(epoch)
          if (done)
            break
          received += value.byteLength
          if (maxBytes !== undefined && received > maxBytes) {
            request.controller.abort()
            throw new ApiError('ERR_FILE_TOO_LARGE', 'Preview exceeds the maximum size')
          }
          chunks.push(value.slice().buffer as ArrayBuffer)
        }
      }
      finally { await reader.cancel().catch(() => {}) }
      return new Blob(chunks)
    }
    catch (e) {
      assertConnection(epoch)
      throw await toApiError(e)
    }
    finally { request.release() }
  }

  async function postBlob(path: string, body: unknown): Promise<Blob> {
    assertConnection(epoch)
    const request = connectionRequest()
    const headers: Record<string, string> = {}
    const csrf = getCsrfToken(path)
    if (csrf)
      headers['X-CSRF-Token'] = csrf
    try {
      const result = await $fetch<Blob>(path, {
        method: 'POST',
        signal: request.controller.signal,
        headers,
        body: body as Record<string, unknown>,
        responseType: 'blob',
      })
      assertConnection(epoch)
      return result
    }
    catch (e) {
      assertConnection(epoch)
      throw await toApiError(e)
    }
    finally { request.release() }
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
      assertConnection(epoch)
      const request = connectionRequest()
      const xhr = new XMLHttpRequest()
      request.controller.signal.addEventListener('abort', () => xhr.abort(), { once: true })
      xhr.onloadend = () => request.release()
      // Without this the request is unreachable once started: cancelling only
      // relabelled the row while the browser kept uploading the whole file.
      if (opts?.signal) {
        if (opts.signal.aborted) {
          request.release()
          reject(new ApiError(ERR_ABORTED, 'Request aborted'))
          return
        }
        opts.signal.addEventListener('abort', () => xhr.abort(), { once: true })
      }
      xhr.open('POST', path)
      const csrf = getCsrfToken(path)
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
          assertConnection(epoch)
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
