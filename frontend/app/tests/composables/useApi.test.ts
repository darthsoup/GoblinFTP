import type { DirectorySizeResult } from '~/types/api'
import { mockNuxtImport } from '@nuxt/test-utils/runtime'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useApi } from '~/composables/useApi'
import { useFileOperations } from '~/composables/useFileOperations'
import { useDirectorySizesStore } from '~/stores/directorySizes'
import { ERR_ABORTED } from '~/types/api'

const { fetch } = vi.hoisted(() => ({ fetch: vi.fn() }))
mockNuxtImport('$fetch', () => fetch)
const total: DirectorySizeResult = { path: '/docs', bytes: 10, hasEntries: true, complete: true, fileCount: 1, directoryCount: 0, skippedCount: 0, calculatedAt: '2026-10-19T12:00:00Z' }

describe('api cancellation and size invalidation', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    fetch.mockResolvedValue({ success: true, data: {} })
    useAuthStore().connected = true
  })

  afterEach(() => {
    useDirectorySizesStore().$dispose()
    vi.unstubAllGlobals()
  })

  it.each([
    new DOMException('cancelled', 'AbortError'),
    Object.assign(new Error('wrapped abort'), { cause: new DOMException('cancelled', 'AbortError') }),
  ])('normalizes fetch aborts without losing the session', async (error) => {
    fetch.mockRejectedValueOnce(error)
    await expect(useApi().post('/api/files/directory-size', { path: '/docs' })).rejects.toMatchObject({ code: ERR_ABORTED })
    expect(useAuthStore().sessionLost).toBe(false)
  })

  it('passes AbortSignal to fetch and normalizes cancellation even if the fetch error has no cause', async () => {
    const controller = new AbortController()
    controller.abort()
    fetch.mockRejectedValueOnce(new Error('request failed'))
    await expect(useApi().post('/api/files/directory-size', { path: '/docs' }, { signal: controller.signal })).rejects.toMatchObject({ code: ERR_ABORTED })
    expect(fetch.mock.calls[0]![1].signal).toBe(controller.signal)
    expect(useAuthStore().sessionLost).toBe(false)
  })

  it.each([
    ['patch', '/api/files/rename'],
    ['patch', '/api/files/copy'],
    ['del', '/api/files'],
    ['post', '/api/files/directory'],
    ['post', '/api/files/write'],
    ['post', '/api/files/upload/commit'],
    ['post', '/api/files/extract'],
    ['post', '/api/files/compress'],
  ] as const)('invalidates sizes around %s %s and rejects stale scan responses', async (method, path) => {
    let resolve!: (response: unknown) => void
    fetch.mockReturnValueOnce(new Promise((done) => {
      resolve = done
    }))
    const sizes = useDirectorySizesStore()
    const pending = sizes.calculate('/docs')
    const signal = fetch.mock.calls[0]![1].signal as AbortSignal
    const mutation = useFileOperations()[method](path, {})
    expect(signal.aborted).toBe(false)
    expect(sizes.entries).toEqual({})
    await mutation
    resolve({ success: true, data: total })
    await pending
    expect(sizes.entries).toEqual({})
  })

  it('preserves cached sizes for reads, download tokens, and upload staging', async () => {
    const sizes = useDirectorySizesStore()
    sizes.entries['/docs'] = { status: 'complete', result: total }
    await useFileOperations().get('/api/files?path=%2F')
    await useFileOperations().post('/api/files/download-token', { path: '/docs/a.txt' })
    await useFileOperations().post('/api/files/upload/reserve', {})
    await useFileOperations().post('/api/files/upload/check', {})
    expect(sizes.get('/docs').status).toBe('complete')
  })

  it('invalidates sizes for multipart uploads before and after the request', async () => {
    const requests: FakeXHR[] = []
    class FakeXHR {
      upload = {}
      status = 200
      response = { success: true, data: {} }
      responseType = ''
      onload?: () => void
      onerror?: () => void
      onabort?: () => void
      open() {}
      setRequestHeader() {}
      send() { requests.push(this) }
      abort() { this.onabort?.() }
    }
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    const sizes = useDirectorySizesStore()
    sizes.entries['/docs'] = { status: 'complete', result: total }
    const pending = useFileOperations().postForm('/api/files/upload', new FormData())
    expect(sizes.entries).toEqual({})
    sizes.entries['/docs'] = { status: 'complete', result: total }
    requests[0]!.onload!()
    await pending
    expect(sizes.entries).toEqual({})
  })

  it('keeps generic transport independent from file-size invalidation', async () => {
    const sizes = useDirectorySizesStore()
    sizes.entries['/docs'] = { status: 'complete', result: total }
    await useApi().post('/api/files/write', {})
    expect(sizes.get('/docs').result).toEqual(total)
  })

  it('invalidates a calculation started while a mutation is in flight, even when the mutation fails', async () => {
    let rejectMutation!: (error: unknown) => void
    fetch.mockReturnValueOnce(new Promise((_resolve, reject) => {
      rejectMutation = reject
    }))
    const mutation = useFileOperations().post('/api/files/write', {})
    let resolveScan!: (response: unknown) => void
    fetch.mockReturnValueOnce(new Promise((resolve) => {
      resolveScan = resolve
    }))
    const sizes = useDirectorySizesStore()
    const scan = sizes.calculate('/docs')
    const signal = fetch.mock.calls[1]![1].signal as AbortSignal
    rejectMutation(new Error('request failed'))
    await expect(mutation).rejects.toThrow()
    expect(signal.aborted).toBe(false)
    resolveScan({ success: true, data: total })
    await scan
    expect(sizes.get('/docs').status).toBe('unknown')
  })

  it.each([false, true])('preserves new-session totals when an old mutation finishes (failed: %s)', async (failed) => {
    let resolveOld!: (response: unknown) => void
    let rejectOld!: (error: unknown) => void
    fetch.mockReturnValueOnce(new Promise((resolve, reject) => {
      resolveOld = resolve
      rejectOld = reject
    }))
    const mutation = useFileOperations().post('/api/files/write', {})
    const sizes = useDirectorySizesStore()
    sizes.$reset()
    sizes.entries['/docs'] = { status: 'complete', result: { ...total, bytes: 200 } }

    if (failed)
      rejectOld(new Error('old connection closed'))
    else
      resolveOld({ success: true, data: {} })
    if (failed)
      await expect(mutation).rejects.toThrow()
    else
      await mutation
    expect(sizes.get('/docs').result?.bytes).toBe(200)
  })

  it('does not cancel a new-session calculation when an old mutation completes', async () => {
    let resolveOld!: (response: unknown) => void
    fetch.mockReturnValueOnce(new Promise((resolve) => {
      resolveOld = resolve
    }))
    const mutation = useFileOperations().patch('/api/files/rename', {})
    const sizes = useDirectorySizesStore()
    sizes.$reset()
    let resolveScan!: (response: unknown) => void
    fetch.mockReturnValueOnce(new Promise((resolve) => {
      resolveScan = resolve
    }))
    const scan = sizes.calculate('/docs')
    const signal = fetch.mock.calls[1]![1].signal as AbortSignal

    resolveOld({ success: true, data: {} })
    await mutation
    expect(signal.aborted).toBe(false)
    expect(sizes.activePath).toBe('/docs')
    resolveScan({ success: true, data: total })
    await scan
    expect(sizes.get('/docs').result).toEqual(total)
  })

  it('preserves new-session totals when an old multipart upload completes', async () => {
    const requests: FakeXHR[] = []
    class FakeXHR {
      upload = {}
      status = 200
      response = { success: true, data: {} }
      responseType = ''
      onload?: () => void
      open() {}
      setRequestHeader() {}
      send() { requests.push(this) }
    }
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    const sizes = useDirectorySizesStore()
    const pending = useFileOperations().postForm('/api/files/upload', new FormData())
    sizes.$reset()
    sizes.entries['/docs'] = { status: 'complete', result: total }
    requests[0]!.onload!()
    await pending
    expect(sizes.get('/docs').result).toEqual(total)
  })
})
