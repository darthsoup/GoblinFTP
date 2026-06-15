import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

describe('useUploadStore', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
    mockApi.get.mockResolvedValue([])
    mockApi.post.mockResolvedValue({})
  })

  it('addEntries builds a nested destPath and keeps the relative path for display', () => {
    const store = useUploadStore()
    const file = new File(['x'], 'a.txt')

    store.addEntries([{ file, relativePath: 'folder/sub/a.txt' }], '/home/user')

    const item = store.items[0]!
    expect(item.destPath).toBe('/home/user/folder/sub/a.txt')
    expect(item.relativePath).toBe('folder/sub/a.txt')
  })

  it('addFiles delegates to addEntries with relativePath = file.name', () => {
    const store = useUploadStore()
    const file = new File(['x'], 'b.txt')

    store.addFiles([file], '/home/') // trailing slash normalized

    const item = store.items[0]!
    expect(item.destPath).toBe('/home/b.txt')
    expect(item.relativePath).toBe('b.txt')
  })
})
