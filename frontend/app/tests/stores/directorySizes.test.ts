import type { DirectorySizeResult } from '~/types/api'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useDirectorySizesStore } from '~/stores/directorySizes'
import { ApiError, ERR_ABORTED } from '~/types/api'

const api = { post: vi.fn(), get: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => api }))

function result(path = '/pub/docs', over: Partial<DirectorySizeResult> = {}): DirectorySizeResult {
  return { path, bytes: 123, hasEntries: true, fileCount: 2, directoryCount: 1, complete: true, skippedCount: 0, calculatedAt: '2026-10-19T12:00:00Z', ...over }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

describe('directory size cache', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.useFakeTimers()
    api.get.mockResolvedValue([])
  })

  afterEach(() => {
    useDirectorySizesStore().$dispose()
    vi.useRealTimers()
  })

  it('retains completed results beyond 60 seconds and explicitly calculates fresh results', async () => {
    const store = useDirectorySizesStore()
    api.post.mockResolvedValue(result('/pub/docs', { bytes: 0 }))
    expect(store.get('/pub/docs').status).toBe('unknown')

    await store.calculate('/pub/docs/')
    expect(api.post).toHaveBeenCalledTimes(1)
    expect(store.get('/pub/docs').result?.bytes).toBe(0)

    await vi.advanceTimersByTimeAsync(120_000)
    expect(store.get('/pub/docs').status).toBe('complete')
    expect(store.get('/pub/docs').result?.bytes).toBe(0)
    await store.calculate('/pub/docs')
    expect(api.post).toHaveBeenCalledTimes(2)
  })

  it('retains partial results as partial and explicitly recalculates cached results', async () => {
    const store = useDirectorySizesStore()
    api.post.mockResolvedValue(result('/pub/docs', { complete: false, skippedCount: 1, reason: 'unreadable' }))
    await store.calculate('/pub/docs')
    expect(store.get('/pub/docs').status).toBe('partial')
    expect(store.get('/pub/docs').result?.reason).toBe('unreadable')

    api.post.mockResolvedValue(result())
    await store.calculate('/pub/docs')
    expect(store.get('/pub/docs').status).toBe('complete')
    expect(api.post).toHaveBeenCalledTimes(2)
  })

  it('deduplicates an active path and waits for its request before switching paths', async () => {
    const first = deferred<DirectorySizeResult>()
    const second = deferred<DirectorySizeResult>()
    api.post.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const store = useDirectorySizesStore()
    const one = store.calculate('/pub/docs')
    const duplicate = store.calculate('/pub/docs')
    expect(api.post).toHaveBeenCalledTimes(1)
    const signal = api.post.mock.calls[0]![2].signal as AbortSignal

    const two = store.calculate('/pub/other')
    expect(signal.aborted).toBe(false)
    expect(api.post).toHaveBeenCalledTimes(1)
    expect(store.get('/pub/docs').status).toBe('unknown')
    first.resolve(result())
    await Promise.all([one, duplicate])
    expect(api.post).toHaveBeenCalledTimes(2)
    expect(store.activePath).toBe('/pub/other')
    expect(store.get('/pub/docs').status).toBe('unknown')

    second.resolve(result('/pub/other'))
    await two
    expect(store.get('/pub/other').status).toBe('complete')
    expect(store.activePath).toBeNull()
  })

  it.each(['cancel', 'invalidate', '$reset'] as const)('ignores a late response after %s', async (method) => {
    const response = deferred<DirectorySizeResult>()
    api.post.mockReturnValue(response.promise)
    const store = useDirectorySizesStore()
    const pending = store.calculate('/pub/docs')
    const signal = api.post.mock.calls[0]![2].signal as AbortSignal
    store[method]()
    expect(signal.aborted).toBe(method !== 'invalidate')
    response.resolve(result())
    await pending
    expect(store.get('/pub/docs').status).toBe('unknown')
    expect(store.activePath).toBeNull()
  })

  it('treats abort as cancellation and retains safe error codes for retry', async () => {
    const store = useDirectorySizesStore()
    api.post.mockRejectedValueOnce(new ApiError(ERR_ABORTED, 'aborted'))
    await store.calculate('/pub/docs')
    expect(store.get('/pub/docs').status).toBe('unknown')

    api.post.mockRejectedValueOnce(new ApiError('ERR_OPERATION_IN_PROGRESS', 'server detail'))
    await store.calculate('/pub/docs')
    expect(store.get('/pub/docs')).toEqual({ status: 'error', errorCode: 'ERR_OPERATION_IN_PROGRESS' })
    api.post.mockResolvedValueOnce(result())
    await store.calculate('/pub/docs')
    expect(store.get('/pub/docs').status).toBe('complete')
  })

  it('preserves the previous result during recalculation, after cancellation and after a late response', async () => {
    const store = useDirectorySizesStore()
    api.post.mockResolvedValueOnce(result())
    await store.calculate('/pub/docs')
    const previous = store.get('/pub/docs').result
    const response = deferred<DirectorySizeResult>()
    api.post.mockReturnValueOnce(response.promise)
    const pending = store.calculate('/pub/docs')
    expect(store.get('/pub/docs')).toMatchObject({ status: 'loading', result: previous })
    store.cancel()
    expect(store.get('/pub/docs')).toEqual({ status: 'complete', result: previous })
    response.resolve(result('/pub/docs', { bytes: 999 }))
    await pending
    expect(store.get('/pub/docs').result).toEqual(previous)
  })

  it('retains complete or partial results after failed and aborted recalculations', async () => {
    const store = useDirectorySizesStore()
    api.post.mockResolvedValueOnce(result('/pub/docs', { complete: false, reason: 'limit' }))
    await store.calculate('/pub/docs')
    const previous = store.get('/pub/docs').result
    api.post.mockRejectedValueOnce(new ApiError('ERR_OPERATION_IN_PROGRESS', 'busy'))
    await store.calculate('/pub/docs')
    expect(store.get('/pub/docs')).toMatchObject({ status: 'error', result: previous, errorCode: 'ERR_OPERATION_IN_PROGRESS' })

    api.post.mockRejectedValueOnce(new ApiError(ERR_ABORTED, 'aborted'))
    await store.calculate('/pub/docs')
    expect(store.get('/pub/docs')).toEqual({ status: 'partial', result: previous })
    store.invalidate()
    expect(store.get('/pub/docs').status).toBe('unknown')
  })

  it('discards navigation results without aborting the request and clears caches on refresh or disconnect', async () => {
    const store = useDirectorySizesStore()
    api.post.mockResolvedValueOnce(result('/cached'))
    await store.calculate('/cached')
    const response = deferred<DirectorySizeResult>()
    api.post.mockReturnValueOnce(response.promise)
    const pending = store.calculate('/pub/docs')
    const signal = api.post.mock.calls[1]![2].signal as AbortSignal
    await useFilesStore().navigate('/elsewhere')
    expect(signal.aborted).toBe(false)
    response.resolve(result())
    await pending
    expect(store.get('/pub/docs').status).toBe('unknown')
    expect(store.get('/cached').status).toBe('complete')

    await useFilesStore().refresh()
    expect(store.get('/cached').status).toBe('unknown')
    api.post.mockResolvedValue(result())
    await store.calculate('/pub/docs')
    await useAuthStore().disconnect()
    expect(store.entries).toEqual({})
  })

  it('refreshes during a scan without aborting its protocol operation or accepting its stale result', async () => {
    const response = deferred<DirectorySizeResult>()
    api.post.mockReturnValueOnce(response.promise)
    const store = useDirectorySizesStore()
    const pending = store.calculate('/pub/docs')
    const signal = api.post.mock.calls[0]![2].signal as AbortSignal
    await useFilesStore().refresh()
    expect(signal.aborted).toBe(false)
    expect(store.activePath).toBeNull()
    response.resolve(result())
    await pending
    expect(store.entries).toEqual({})
  })

  it('waits for invalidated work before requesting another calculation', async () => {
    const first = deferred<DirectorySizeResult>()
    const second = deferred<DirectorySizeResult>()
    api.post.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const store = useDirectorySizesStore()
    const one = store.calculate('/pub/docs')
    const signal = api.post.mock.calls[0]![2].signal as AbortSignal
    store.invalidate()
    const two = store.calculate('/pub/other')
    expect(signal.aborted).toBe(false)
    expect(api.post).toHaveBeenCalledTimes(1)
    first.resolve(result())
    await one
    expect(api.post).toHaveBeenCalledTimes(2)
    expect(store.get('/pub/docs').status).toBe('unknown')
    second.resolve(result('/pub/other'))
    await two
    expect(store.get('/pub/other').status).toBe('complete')
  })

  it('does not abort invalidated work when a queued calculation is cancelled', async () => {
    const response = deferred<DirectorySizeResult>()
    api.post.mockReturnValueOnce(response.promise)
    const store = useDirectorySizesStore()
    const one = store.calculate('/pub/docs')
    const signal = api.post.mock.calls[0]![2].signal as AbortSignal
    const two = store.calculate('/pub/other')
    store.cancel()
    expect(signal.aborted).toBe(false)
    response.resolve(result())
    await Promise.all([one, two])
    expect(api.post).toHaveBeenCalledTimes(1)
    expect(store.activePath).toBeNull()
    expect(store.get('/pub/other').status).toBe('unknown')
  })

  it('aborts invalidated requests when the connection resets', async () => {
    const response = deferred<DirectorySizeResult>()
    api.post.mockReturnValueOnce(response.promise)
    const store = useDirectorySizesStore()
    const pending = store.calculate('/pub/docs')
    const signal = api.post.mock.calls[0]![2].signal as AbortSignal
    store.invalidate()
    expect(signal.aborted).toBe(false)
    store.$reset()
    expect(signal.aborted).toBe(true)
    response.resolve(result())
    await pending
    expect(store.entries).toEqual({})
  })

  it('drops a queued calculation when its results are invalidated before it starts', async () => {
    const response = deferred<DirectorySizeResult>()
    api.post.mockReturnValueOnce(response.promise)
    const store = useDirectorySizesStore()
    const one = store.calculate('/pub/docs')
    const two = store.calculate('/pub/other')
    store.invalidate()
    response.resolve(result())
    await Promise.all([one, two])
    expect(api.post).toHaveBeenCalledTimes(1)
    expect(store.entries).toEqual({})
    expect(store.activePath).toBeNull()
  })
})
