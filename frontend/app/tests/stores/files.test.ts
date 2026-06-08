import type { FileInfo } from '~/types/api'
import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

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
})
