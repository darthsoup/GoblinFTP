import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

// Every paste here overwrites an existing destination, so the delete runs first.
const conflictChoice = vi.fn(async () => 'overwrite')
vi.mock('~/stores/modal', async (orig) => {
  const actual = await orig<typeof import('~/stores/modal')>()
  return { ...actual, useModalStore: () => ({ pasteConflict: conflictChoice }) }
})

describe('filesStore.paste overwrite failures', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
    mockApi.get.mockResolvedValue({ files: [], path: '/dest' })
  })

  // DELETE answers 200 with per-item outcomes, so a refused delete does not
  // throw. The move ran anyway and the user was told only "paste failed".
  it('aborts the move when the destination delete was refused', async () => {
    const store = useFilesStore()
    // Cut from /src, paste into /dest where a file of the same name exists, so
    // the overwrite path runs the delete before the move.
    store.currentPath = '/src'
    store.cutToClipboard(['a.txt'])
    store.currentPath = '/dest'
    store.files = [{ name: 'a.txt', size: 1, isDir: false, modified: '', mode: '-rw-r--r--' }]

    mockApi.del.mockResolvedValue({
      deleted: [],
      failed: [{ path: '/dest/a.txt', code: 'ERR_FILE_PERMISSION', message: 'Permission denied by the server.' }],
    })

    const res = await store.paste()

    expect(mockApi.patch).not.toHaveBeenCalled()
    expect(res.failures).toHaveLength(1)
    expect(res.failures[0]!.code).toBe('ERR_FILE_PERMISSION')
  })

  it('reports a distinct code when the destination was removed and the move then failed', async () => {
    const store = useFilesStore()
    // Cut from /src, paste into /dest where a file of the same name exists, so
    // the overwrite path runs the delete before the move.
    store.currentPath = '/src'
    store.cutToClipboard(['a.txt'])
    store.currentPath = '/dest'
    store.files = [{ name: 'a.txt', size: 1, isDir: false, modified: '', mode: '-rw-r--r--' }]

    mockApi.del.mockResolvedValue({ deleted: ['/dest/a.txt'], failed: [] })
    mockApi.patch.mockRejectedValue(new Error('move failed'))

    const res = await store.paste()

    expect(res.failures).toHaveLength(1)
    expect(res.failures[0]!.code).toBe('ERR_OVERWRITE_LOST_DESTINATION')
  })
})
