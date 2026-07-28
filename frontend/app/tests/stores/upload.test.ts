import type { UploadConflict } from '~/types/api'
import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '~/types/api'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

const CHECK = '/api/files/upload/check'
const UPLOAD = '/api/files/upload'

function conflict(path: string, over: Partial<UploadConflict> = {}): UploadConflict {
  const name = path.split('/').pop()!
  return { path, name, suggestedName: `x (1).txt`, size: 1, isDir: false, modified: '', ...over }
}

// Routes each POST by URL so a test only has to describe the pre-flight result.
function routePosts(conflicts: UploadConflict[] = [], onUpload?: (body: unknown) => unknown) {
  mockApi.post.mockImplementation((url: string, body: unknown) => {
    if (url === CHECK)
      return Promise.resolve({ conflicts })
    if (onUpload)
      return Promise.resolve(onUpload(body))
    return Promise.resolve({})
  })
}

function uploadCalls() {
  return mockApi.post.mock.calls.filter(c => c[0] === UPLOAD)
}

describe('useUploadStore', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
    mockApi.get.mockResolvedValue([])
    routePosts()
  })

  it('addEntries builds a nested destPath and keeps the relative path for display', async () => {
    const store = useUploadStore()
    const file = new File(['x'], 'a.txt')

    await store.addEntries([{ file, relativePath: 'folder/sub/a.txt' }], '/home/user')

    const item = store.items[0]!
    expect(item.destPath).toBe('/home/user/folder/sub/a.txt')
    expect(item.relativePath).toBe('folder/sub/a.txt')
  })

  it('addFiles delegates to addEntries with relativePath = file.name', async () => {
    const store = useUploadStore()
    const file = new File(['x'], 'b.txt')

    await store.addFiles([file], '/home/') // trailing slash normalized

    const item = store.items[0]!
    expect(item.destPath).toBe('/home/b.txt')
    expect(item.relativePath).toBe('b.txt')
  })

  it('checks every destination before uploading and enqueues when nothing conflicts', async () => {
    const store = useUploadStore()
    const modal = useModalStore()
    const spy = vi.spyOn(modal, 'uploadConflict')

    await store.addEntries([
      { file: new File(['x'], 'a.txt'), relativePath: 'a.txt' },
      { file: new File(['y'], 'b.txt'), relativePath: 'b.txt' },
    ], '/d')

    expect(mockApi.post).toHaveBeenCalledWith(CHECK, { paths: ['/d/a.txt', '/d/b.txt'] })
    expect(spy).not.toHaveBeenCalled()
    expect(store.items).toHaveLength(2)
  })

  it('skip leaves the remote file untouched', async () => {
    routePosts([conflict('/d/a.txt')])
    const store = useUploadStore()
    vi.spyOn(useModalStore(), 'uploadConflict').mockResolvedValue({
      kind: 'resolve',
      decisions: { '/d/a.txt': 'skip' },
    })

    await store.addEntries([{ file: new File(['x'], 'a.txt'), relativePath: 'a.txt' }], '/d')

    expect(store.items[0]!.status).toBe('skipped')
    expect(uploadCalls()).toHaveLength(0)
  })

  // Decisions must be applied through items.value, not the raw objects passed to
  // addEntries — mutating those bypasses the reactive proxy and the queue panel
  // keeps showing "Checking" forever.
  it('applies decisions to the reactive items, not the local copies', async () => {
    routePosts([conflict('/d/a.txt')])
    const store = useUploadStore()
    vi.spyOn(useModalStore(), 'uploadConflict').mockResolvedValue({
      kind: 'resolve',
      decisions: { '/d/a.txt': 'skip' },
    })

    await store.addEntries([{ file: new File(['x'], 'a.txt'), relativePath: 'a.txt' }], '/d')

    expect(store.items.map(i => i.status)).toEqual(['skipped'])
    expect(store.hasActive).toBe(false)
  })

  it('rename rewrites the last segment of both destPath and relativePath', async () => {
    routePosts([conflict('/d/sub/a.txt', { suggestedName: 'a (1).txt' })])
    const store = useUploadStore()
    vi.spyOn(useModalStore(), 'uploadConflict').mockResolvedValue({
      kind: 'resolve',
      decisions: { '/d/sub/a.txt': 'rename' },
    })

    await store.addEntries([{ file: new File(['x'], 'a.txt'), relativePath: 'sub/a.txt' }], '/d')

    const item = store.items[0]!
    expect(item.destPath).toBe('/d/sub/a (1).txt')
    expect(item.relativePath).toBe('sub/a (1).txt')
  })

  it('overwrite sends the consent flag with the upload', async () => {
    routePosts([conflict('/d/a.txt')])
    const store = useUploadStore()
    vi.spyOn(useModalStore(), 'uploadConflict').mockResolvedValue({
      kind: 'resolve',
      decisions: { '/d/a.txt': 'overwrite' },
    })

    await store.addEntries([{ file: new File(['x'], 'a.txt'), relativePath: 'a.txt' }], '/d')
    await vi.waitFor(() => expect(uploadCalls()).toHaveLength(1))

    const form = uploadCalls()[0]![1] as FormData
    expect(form.get('overwrite')).toBe('true')
  })

  it('cancel drops the whole batch without uploading anything', async () => {
    routePosts([conflict('/d/a.txt')])
    const store = useUploadStore()
    vi.spyOn(useModalStore(), 'uploadConflict').mockResolvedValue({ kind: 'cancel' })

    await store.addEntries([
      { file: new File(['x'], 'a.txt'), relativePath: 'a.txt' },
      { file: new File(['y'], 'b.txt'), relativePath: 'b.txt' },
    ], '/d')

    expect(store.items.every(i => i.status === 'cancelled')).toBe(true)
    expect(uploadCalls()).toHaveLength(0)
  })

  // The pre-flight is an optimization; the backend guard is the real protection,
  // so losing the pre-flight must not stop the upload.
  it('still uploads when the pre-flight fails', async () => {
    mockApi.post.mockImplementation((url: string) => {
      if (url === CHECK)
        return Promise.reject(new ApiError('ERR_OPERATION_FAILED', 'nope'))
      return Promise.resolve({})
    })
    const store = useUploadStore()

    await store.addEntries([{ file: new File(['x'], 'a.txt'), relativePath: 'a.txt' }], '/d')
    await vi.waitFor(() => expect(store.items[0]!.status).toBe('done'))

    expect(uploadCalls()).toHaveLength(1)
  })

  it('does not retry a conflict, and re-prompts instead', async () => {
    let uploads = 0
    mockApi.post.mockImplementation((url: string, body: unknown) => {
      if (url === CHECK) {
        // Free at pre-flight, taken by the time the bytes arrive.
        return Promise.resolve({ conflicts: uploads === 0 ? [] : [conflict('/d/a.txt', { suggestedName: 'a (1).txt' })] })
      }
      if (url === UPLOAD) {
        uploads++
        const form = body as FormData
        if (form.get('overwrite') !== 'true')
          return Promise.reject(new ApiError('ERR_FILE_EXISTS', 'exists'))
      }
      return Promise.resolve({})
    })
    const store = useUploadStore()
    vi.spyOn(useModalStore(), 'uploadConflict').mockResolvedValue({
      kind: 'resolve',
      decisions: { '/d/a.txt': 'overwrite' },
    })

    await store.addEntries([{ file: new File(['x'], 'a.txt'), relativePath: 'a.txt' }], '/d')
    await vi.waitFor(() => expect(store.items[0]!.status).toBe('done'))

    // One rejected attempt + one consented retry: no 5x backoff storm.
    expect(uploads).toBe(2)
  })

  it('records the error code so the row can localize the failure', async () => {
    mockApi.post.mockImplementation((url: string) => {
      if (url === CHECK)
        return Promise.resolve({ conflicts: [] })
      return Promise.reject(new ApiError('ERR_FILE_PERMISSION', 'denied'))
    })
    const store = useUploadStore()

    await store.addEntries([{ file: new File(['x'], 'a.txt'), relativePath: 'a.txt' }], '/d')
    await vi.waitFor(() => expect(store.items[0]!.status).toBe('error'))

    expect(store.items[0]!.errorCode).toBe('ERR_FILE_PERMISSION')
  })

  // b.txt is free at pre-flight and only conflicts once its bytes are sent. The
  // batch-wide "apply to all" must settle it silently rather than interrupting.
  it('applyToAll resolves a raced conflict without opening the dialog again', async () => {
    let uploads = 0
    mockApi.post.mockImplementation((url: string, body: unknown) => {
      if (url === CHECK) {
        const { paths } = body as { paths: string[] }
        // Batch pre-flight flags only a.txt; the later single-path re-check
        // (triggered by b.txt's 409) reports b.txt as taken.
        return Promise.resolve({
          conflicts: paths.length > 1 ? [conflict('/d/a.txt')] : [conflict(paths[0]!)],
        })
      }
      if (url === UPLOAD) {
        uploads++
        const form = body as FormData
        if (form.get('overwrite') !== 'true')
          return Promise.reject(new ApiError('ERR_FILE_EXISTS', 'exists'))
      }
      return Promise.resolve({})
    })
    const store = useUploadStore()
    const spy = vi.spyOn(useModalStore(), 'uploadConflict').mockResolvedValue({
      kind: 'resolve',
      decisions: { '/d/a.txt': 'overwrite' },
      applyToAll: 'overwrite',
    })

    await store.addEntries([
      { file: new File(['x'], 'a.txt'), relativePath: 'a.txt' },
      { file: new File(['y'], 'b.txt'), relativePath: 'b.txt' },
    ], '/d')
    await vi.waitFor(() => expect(store.items.every(i => i.status === 'done')).toBe(true))

    // Only the batch dialog — the raced conflict reused the decision.
    expect(spy).toHaveBeenCalledTimes(1)
    // a.txt once (consented), b.txt rejected then retried with consent.
    expect(uploads).toBe(3)
  })
})
