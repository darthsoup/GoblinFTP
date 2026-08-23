import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '~/types/api'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn(), postForm: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

const CHECK = '/api/files/upload/check'
const RESERVE = '/api/files/upload/reserve'
const COMMIT = '/api/files/upload/commit'
const ABORT = '/api/files/upload/abort'
const CHUNK = '/api/files/upload/chunk'

// The store splits on file.size > chunkSize, which comes from systemVars. A tiny chunk
// size exercises the multi-chunk path every other fixture is too small to reach.
const CHUNK_SIZE = 4

function bigFile(name = 'big.bin', size = 10) {
  return new File(['x'.repeat(size)], name)
}

function postsFor(overrides: Record<string, (body: unknown) => unknown> = {}) {
  mockApi.post.mockImplementation((url: string, body: unknown) => {
    const handler = overrides[url]
    if (handler)
      return Promise.resolve(handler(body))
    if (url === CHECK)
      return Promise.resolve({ conflicts: [] })
    if (url === RESERVE)
      return Promise.resolve({ uploadId: 'upload-1' })
    return Promise.resolve({})
  })
}

function chunkCalls() {
  return mockApi.postForm.mock.calls.filter(c => c[0] === CHUNK)
}

function postCalls(url: string) {
  return mockApi.post.mock.calls.filter(c => c[0] === url)
}

describe('chunked upload', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
    mockApi.get.mockResolvedValue([])
    postsFor()
    mockApi.postForm.mockResolvedValue({})
    const auth = useAuthStore()
    auth.systemVars = { ...(auth.systemVars ?? {}), upload: { chunkSize: CHUNK_SIZE, maxConcurrent: 1 } } as never
  })

  it('reserves, sends every chunk, then commits', async () => {
    const store = useUploadStore()
    await store.addEntries([{ file: bigFile('big.bin', 10), relativePath: 'big.bin' }], '/d')
    await vi.waitFor(() => expect(store.items[0]!.status).toBe('done'))

    expect(postCalls(RESERVE)).toHaveLength(1)
    // ceil(10 / 4) = 3
    expect(chunkCalls()).toHaveLength(3)
    expect(postCalls(COMMIT)).toHaveLength(1)
  })

  // Regression: retryItem reset every field but uploadId, which _uploadChunked reads as
  // "already staged", so a retry committed a partial set and the item never recovered.
  it('retry after a mid-transfer failure re-uploads instead of committing a partial set', async () => {
    const store = useUploadStore()
    let failChunks = true
    mockApi.postForm.mockImplementation((url: string) => {
      if (url === CHUNK && failChunks)
        return Promise.reject(new ApiError('ERR_FILE_PERMISSION', 'denied'))
      return Promise.resolve({})
    })

    await store.addEntries([{ file: bigFile('big.bin', 10), relativePath: 'big.bin' }], '/d')
    await vi.waitFor(() => expect(store.items[0]!.status).toBe('error'))

    // The failed attempt must not leave chunks stranded on the server.
    expect(postCalls(ABORT).length).toBeGreaterThan(0)
    expect(store.items[0]!.uploadId).toBeUndefined()

    failChunks = false
    mockApi.postForm.mockClear()
    mockApi.post.mockClear()
    postsFor()

    await store.retryItem(store.items[0]!.id)
    await vi.waitFor(() => expect(store.items[0]!.status).toBe('done'))

    expect(chunkCalls()).toHaveLength(3)
    expect(postCalls(RESERVE)).toHaveLength(1)
  })

  // Regression: the error path never released staged chunks, and nothing sweeps the
  // staging area server-side, so every failed large upload leaked bytes onto the volume.
  it('a failed upload releases its staged chunks', async () => {
    const store = useUploadStore()
    mockApi.postForm.mockImplementation((url: string) => {
      if (url === CHUNK)
        return Promise.reject(new ApiError('ERR_QUOTA_EXCEEDED', 'full'))
      return Promise.resolve({})
    })

    await store.addEntries([{ file: bigFile('big.bin', 10), relativePath: 'big.bin' }], '/d')
    await vi.waitFor(() => expect(store.items[0]!.status).toBe('error'))

    const aborts = postCalls(ABORT)
    expect(aborts).toHaveLength(1)
    expect((aborts[0]![1] as { uploadId: string }).uploadId).toBe('upload-1')
  })

  it('$reset releases chunks still staged for in-flight items', async () => {
    const store = useUploadStore()
    let release: (() => void) | undefined
    mockApi.postForm.mockImplementation((url: string) => {
      if (url === CHUNK)
        return new Promise<unknown>((resolve) => { release = () => resolve({}) })
      return Promise.resolve({})
    })

    await store.addEntries([{ file: bigFile('big.bin', 10), relativePath: 'big.bin' }], '/d')
    await vi.waitFor(() => expect(store.items[0]!.uploadId).toBe('upload-1'))

    store.$reset()
    await vi.waitFor(() => expect(postCalls(ABORT)).toHaveLength(1))
    release?.()
  })
})
