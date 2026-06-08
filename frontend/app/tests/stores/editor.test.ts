import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

describe('useEditorStore', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('opens a file and sets it as active', async () => {
    mockApi.get.mockResolvedValue({ content: 'hello', path: '/a.txt' })
    const store = useEditorStore()

    await store.openFile('/a.txt')

    expect(store.tabs).toHaveLength(1)
    expect(store.tabs[0]!.path).toBe('/a.txt')
    expect(store.activeTab?.content).toBe('hello')
  })

  it('closeTab removes the tab', async () => {
    mockApi.get.mockResolvedValue({ content: '', path: '/a.txt' })
    const store = useEditorStore()
    await store.openFile('/a.txt')
    const id = store.tabs[0]!.id

    store.closeTab(id)

    expect(store.tabs).toHaveLength(0)
    expect(store.activeTab).toBeNull()
  })

  it('restores tabs from a saved manifest', async () => {
    localStorage.setItem('gftp_editor_tabs', JSON.stringify({ paths: ['/a.txt', '/b.txt'], activePath: '/b.txt' }))
    mockApi.get.mockResolvedValue({ content: '', path: '/x' })
    const store = useEditorStore()

    await store.restore()

    expect(store.tabs.map(t => t.path)).toEqual(['/a.txt', '/b.txt'])
    expect(store.activeTab?.path).toBe('/b.txt')
  })

  it('drops tabs that fail to reload during restore', async () => {
    localStorage.setItem('gftp_editor_tabs', JSON.stringify({ paths: ['/gone.txt'], activePath: '/gone.txt' }))
    mockApi.get.mockRejectedValue(new Error('not found'))
    const store = useEditorStore()

    await store.restore()

    expect(store.tabs).toHaveLength(0)
  })
})
