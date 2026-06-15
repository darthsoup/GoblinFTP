import type { FileInfo } from '~/types/api'
import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '~/types/api'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

describe('useFilesStore', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
  })

  it('lists files and updates state', async () => {
    const files: FileInfo[] = [{ name: 'a.txt', size: 10, isDir: false, modified: '2024-01-01T00:00:00Z', mode: '-rw-r--r--' }]
    mockApi.get.mockResolvedValue(files)
    const store = useFilesStore()

    await store.list('/home')

    expect(store.files).toEqual(files)
    expect(store.currentPath).toBe('/home')
    expect(store.loading).toBe(false)
  })

  it('toggleSelection adds and removes names', () => {
    const store = useFilesStore()
    store.toggleSelection('a.txt')
    expect(store.selected.has('a.txt')).toBe(true)
    store.toggleSelection('a.txt')
    expect(store.selected.has('a.txt')).toBe(false)
  })

  it('startRename/cancelRename track the in-place edit target', () => {
    const store = useFilesStore()
    expect(store.editingName).toBeNull()
    store.startRename('a.txt')
    expect(store.editingName).toBe('a.txt')
    store.cancelRename()
    expect(store.editingName).toBeNull()
  })

  it('listing clears any in-progress rename', async () => {
    mockApi.get.mockResolvedValue([])
    const store = useFilesStore()
    store.startRename('a.txt')
    await store.list('/home')
    expect(store.editingName).toBeNull()
  })

  it('copyToClipboard/cutToClipboard capture the source dir and names', async () => {
    mockApi.get.mockResolvedValue([])
    const store = useFilesStore()
    await store.list('/home')
    store.copyToClipboard(['a.txt', 'b.txt'])
    expect(store.clipboard).toEqual({ mode: 'copy', sourcePath: '/home', names: ['a.txt', 'b.txt'] })
    store.cutToClipboard(['c.txt'])
    expect(store.clipboard).toEqual({ mode: 'cut', sourcePath: '/home', names: ['c.txt'] })
    store.clearClipboard()
    expect(store.clipboard).toBeNull()
  })

  it('paste with no conflict copies each item then refreshes', async () => {
    mockApi.get.mockResolvedValue([])
    mockApi.patch.mockResolvedValue(undefined)
    const store = useFilesStore()
    await store.list('/src')
    store.copyToClipboard(['a.txt'])
    await store.list('/dst') // navigate to a different, empty dir → no collision
    mockApi.get.mockClear()

    const res = await store.paste()

    expect(res).toEqual({ mode: 'copy', ok: 1, failed: 0, failures: [] })
    expect(mockApi.patch).toHaveBeenCalledWith('/api/files/copy', { from: '/src/a.txt', to: '/dst/a.txt' })
    expect(mockApi.get).toHaveBeenCalled() // list() refresh
    expect(store.clipboard).not.toBeNull() // copy keeps the clipboard
  })

  it('paste of a cut moves via rename and clears the clipboard', async () => {
    mockApi.get.mockResolvedValue([])
    mockApi.patch.mockResolvedValue(undefined)
    const store = useFilesStore()
    await store.list('/src')
    store.cutToClipboard(['a.txt'])
    await store.list('/dst')

    const res = await store.paste()

    expect(res).toEqual({ mode: 'cut', ok: 1, failed: 0, failures: [] })
    expect(mockApi.patch).toHaveBeenCalledWith('/api/files/rename', { from: '/src/a.txt', to: '/dst/a.txt' })
    expect(store.clipboard).toBeNull() // cut clears the clipboard
  })

  it('deleteFiles returns the batch result (deleted + classified failures)', async () => {
    const result = { deleted: ['/a.txt'], failed: [{ path: '/b', code: 'ERR_DIR_NOT_EMPTY', message: 'not empty' }] }
    mockApi.del.mockResolvedValue(result)
    mockApi.get.mockResolvedValue([]) // list() refresh
    const store = useFilesStore()

    const res = await store.deleteFiles(['/a.txt', '/b'])

    expect(res).toEqual(result)
    expect(mockApi.del).toHaveBeenCalledWith('/api/files', { paths: ['/a.txt', '/b'] })
  })

  it('paste collects per-item failures with code + message', async () => {
    mockApi.get.mockResolvedValue([]) // empty dirs → no conflicts
    const store = useFilesStore()
    await store.list('/src')
    store.copyToClipboard(['a.txt', 'b.txt'])
    await store.list('/dst')
    mockApi.patch.mockImplementation((_url: string, body: { from: string }) =>
      body.from.endsWith('b.txt')
        ? Promise.reject(new ApiError('ERR_FILE_PERMISSION', 'Permission denied by the server'))
        : Promise.resolve(undefined),
    )

    const res = await store.paste()

    expect(res.ok).toBe(1)
    expect(res.failed).toBe(1)
    expect(res.failures).toEqual([
      { path: '/dst/b.txt', code: 'ERR_FILE_PERMISSION', message: 'Permission denied by the server' },
    ])
  })
})
