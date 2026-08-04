import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '~/types/api'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

function readResult(content = 'hello', version: string | null = '5:1000') {
  return { content, path: '/a.txt', version, size: content.length, modified: '2024-01-01T00:00:00Z' }
}

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

// The token must survive open -> save -> save: the server refuses a write whose
// expectedVersion is stale, so a tab that fails to adopt the refreshed version
// would conflict with the write it just made.
describe('useEditorStore conflict detection', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
    localStorage.clear()
  })

  async function openTab(version: string | null = '5:1000') {
    mockApi.get.mockResolvedValue(readResult('hello', version))
    const store = useEditorStore()
    await store.openFile('/a.txt')
    return { store, id: store.tabs[0]!.id }
  }

  it('sends the version captured at open as expectedVersion', async () => {
    const { store, id } = await openTab()
    mockApi.post.mockResolvedValue({ version: '9:2000', size: 9, modified: '2024-01-02T00:00:00Z' })

    store.updateContent(id, 'changed')
    await store.saveTab(id)

    expect(mockApi.post).toHaveBeenCalledWith('/api/files/write', {
      path: '/a.txt',
      content: 'changed',
      expectedVersion: '5:1000',
    })
  })

  it('adopts the refreshed version returned by a successful save', async () => {
    const { store, id } = await openTab()
    mockApi.post.mockResolvedValue({ version: '9:2000', size: 9, modified: '2024-01-02T00:00:00Z' })

    store.updateContent(id, 'changed')
    await store.saveTab(id)

    expect(store.tabs[0]!.version).toBe('9:2000')
    expect(store.tabs[0]!.baselineSize).toBe(9)
  })

  it('a null version means no detection is available, so the save forces', async () => {
    const { store, id } = await openTab(null)
    mockApi.post.mockResolvedValue({ version: null, size: 0, modified: '' })

    store.updateContent(id, 'changed')
    await store.saveTab(id)

    expect(mockApi.post).toHaveBeenCalledWith('/api/files/write', {
      path: '/a.txt',
      content: 'changed',
      overwrite: true,
    })
  })

  it('marks the tab on ERR_FILE_MODIFIED rather than surfacing a raw error', async () => {
    const { store, id } = await openTab()
    mockApi.post.mockRejectedValue(new ApiError('ERR_FILE_MODIFIED', 'changed on the server'))

    store.updateContent(id, 'changed')
    await store.saveTab(id)

    expect(store.tabs[0]!.conflict).toBe('modified')
    expect(store.tabs[0]!.error).toBeUndefined()
    // The buffer is untouched, so the user's work is still recoverable.
    expect(store.tabs[0]!.content).toBe('changed')
  })

  it('marks the tab as deleted on ERR_FILE_NOT_FOUND', async () => {
    const { store, id } = await openTab()
    mockApi.post.mockRejectedValue(new ApiError('ERR_FILE_NOT_FOUND', 'gone'))

    store.updateContent(id, 'changed')
    await store.saveTab(id)

    expect(store.tabs[0]!.conflict).toBe('deleted')
  })

  // Positive control for the test below: proves autosave really does fire in
  // this harness, so "not called" there is the guard and not a dead timer.
  it('autosave fires on a clean tab', async () => {
    const { store, id } = await openTab()
    mockApi.post.mockResolvedValue({ version: '9:2000', size: 9, modified: '' })
    useSettingsStore().editorAutoSave = true

    vi.useFakeTimers()
    store.updateContent(id, 'changed')
    await vi.advanceTimersByTimeAsync(5000)
    vi.useRealTimers()

    expect(mockApi.post).toHaveBeenCalledWith('/api/files/write', expect.anything())
  })

  // Without this the tab would retry on every autosave tick, hammering the
  // server with a request that cannot succeed.
  it('an unresolved conflict blocks the next autosave', async () => {
    const { store, id } = await openTab()
    mockApi.post.mockRejectedValue(new ApiError('ERR_FILE_MODIFIED', 'changed'))
    store.updateContent(id, 'changed')
    await store.saveTab(id)
    expect(store.tabs[0]!.conflict).toBe('modified')

    vi.useFakeTimers()
    useSettingsStore().editorAutoSave = true
    mockApi.post.mockClear()
    store.updateContent(id, 'changed again')
    await vi.advanceTimersByTimeAsync(5000)
    vi.useRealTimers()

    expect(mockApi.post).not.toHaveBeenCalled()
  })

  it('force skips the precondition and clears the conflict', async () => {
    const { store, id } = await openTab()
    mockApi.post.mockRejectedValueOnce(new ApiError('ERR_FILE_MODIFIED', 'changed'))
    store.updateContent(id, 'mine')
    await store.saveTab(id)

    mockApi.post.mockResolvedValue({ version: '4:3000', size: 4, modified: '2024-01-03T00:00:00Z' })
    await store.saveTab(id, { force: true })

    expect(mockApi.post).toHaveBeenLastCalledWith('/api/files/write', {
      path: '/a.txt',
      content: 'mine',
      overwrite: true,
    })
    expect(store.tabs[0]!.conflict).toBeUndefined()
    expect(store.tabs[0]!.version).toBe('4:3000')
  })

  // The revision bump is what makes EditorPane rebuild its CodeMirror state.
  // Without it the pane keeps showing the document the user just discarded and
  // the next save writes it back, which is the loss this feature prevents.
  it('reloadTab replaces the buffer and bumps the revision', async () => {
    const { store, id } = await openTab()
    mockApi.post.mockRejectedValue(new ApiError('ERR_FILE_MODIFIED', 'changed'))
    store.updateContent(id, 'mine')
    await store.saveTab(id)
    const before = store.tabs[0]!.revision

    mockApi.get.mockResolvedValue(readResult('theirs', '6:4000'))
    await store.reloadTab(id)

    expect(store.tabs[0]!.content).toBe('theirs')
    expect(store.tabs[0]!.savedContent).toBe('theirs')
    expect(store.tabs[0]!.version).toBe('6:4000')
    expect(store.tabs[0]!.conflict).toBeUndefined()
    expect(store.tabs[0]!.revision).toBe(before + 1)
  })

  // Mod-S on a tab whose read failed must not write the empty buffer over a
  // healthy file. There is no baseline, so there is nothing to save against.
  it('a tab with no baseline never saves', async () => {
    mockApi.get.mockRejectedValue(new Error('boom'))
    const store = useEditorStore()
    await store.openFile('/a.txt')

    await store.saveTab(store.tabs[0]!.id)

    expect(mockApi.post).not.toHaveBeenCalled()
  })
})
