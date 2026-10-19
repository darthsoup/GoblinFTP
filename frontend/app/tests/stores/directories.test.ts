import type { FileInfo } from '~/types/api'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, ERR_ABORTED } from '~/types/api'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

function directory(name: string): FileInfo {
  return { name, isDir: true, size: 0, modified: '', mode: '' }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

describe('directory listings', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.resetAllMocks()
    mockApi.get.mockResolvedValue([])
  })

  it('caches tree listings but fetches a fresh main-pane listing', async () => {
    const store = useDirectoriesStore()
    mockApi.get.mockResolvedValue([directory('docs')])
    await store.loadTree('/')
    await store.loadTree('/')
    expect(mockApi.get).toHaveBeenCalledTimes(1)
    await useFilesStore().list('/')
    expect(mockApi.get).toHaveBeenCalledTimes(2)
    expect(store.listings['/']?.files).toEqual([directory('docs')])
  })

  it('deduplicates a branch listing with foreground navigation', async () => {
    const pending = deferred<FileInfo[]>()
    mockApi.get.mockReturnValue(pending.promise)
    const directories = useDirectoriesStore()
    const tree = directories.loadTree('/docs')
    await Promise.resolve()
    const navigation = useFilesStore().navigate('/docs')
    pending.resolve([])
    await Promise.all([tree, navigation])
    expect(mockApi.get).toHaveBeenCalledTimes(1)
    expect(useFilesStore().currentPath).toBe('/docs')
  })

  it('allows only one tree request at a time and lets foreground requests bypass the queue', async () => {
    const first = deferred<FileInfo[]>()
    mockApi.get.mockImplementation((url: string) => url.endsWith('%2Fa') ? first.promise : Promise.resolve([]))
    const store = useDirectoriesStore()
    const a = store.loadTree('/a')
    const b = store.loadTree('/b')
    await Promise.resolve()
    expect(mockApi.get).toHaveBeenCalledTimes(1)
    await store.load('/foreground', true)
    expect(mockApi.get).toHaveBeenCalledTimes(2)
    first.resolve([])
    await Promise.all([a, b])
    expect(mockApi.get).toHaveBeenLastCalledWith('/api/files?path=%2Fb')
  })

  it('does not change navigation, selection, editing, or history when loading a branch', async () => {
    const files = useFilesStore()
    await files.navigate('/home')
    files.setSelection(['report.txt'])
    files.startRename('report.txt')
    await useDirectoriesStore().loadTree('/other')
    expect(files.currentPath).toBe('/home')
    expect([...files.selected]).toEqual(['report.txt'])
    expect(files.editingName).toBe('report.txt')
    expect(files.canGoBack).toBe(false)
  })

  it('distinguishes an empty directory from an unreadable one and supports retry', async () => {
    const store = useDirectoriesStore()
    mockApi.get.mockRejectedValueOnce(new ApiError('ERR_FILE_PERMISSION', 'private'))
    await expect(store.loadTree('/private')).rejects.toThrow('private')
    expect(store.listings['/private']).toMatchObject({ files: null, loading: false, errorCode: 'ERR_FILE_PERMISSION' })
    await store.loadTree('/private')
    expect(store.listings['/private']).toMatchObject({ files: [], dirty: false, errorCode: null })
    await store.loadTree('/private')
    expect(mockApi.get).toHaveBeenCalledTimes(2)
  })

  it('preserves invalidation when an older listing finishes after a mutation', async () => {
    const pending = deferred<FileInfo[]>()
    mockApi.get.mockReturnValue(pending.promise)
    const store = useDirectoriesStore()
    const listing = store.load('/home')
    store.invalidate(['/home/new'])
    pending.resolve([])
    await listing
    expect(store.listings['/home']?.dirty).toBe(true)
    mockApi.get.mockResolvedValue([directory('new')])
    await store.loadTree('/home')
    expect(store.listings['/home']?.dirty).toBe(false)
  })

  it('retries a failed refresh even when a previous successful snapshot exists', async () => {
    const store = useDirectoriesStore()
    await store.load('/home')
    mockApi.get.mockRejectedValueOnce(new ApiError('ERR_FILE_PERMISSION', 'denied'))
    await expect(store.load('/home', true)).rejects.toThrow('denied')
    mockApi.get.mockResolvedValue([directory('new')])
    await store.loadTree('/home')
    expect(store.listings['/home']).toMatchObject({ files: [directory('new')], errorCode: null, dirty: false })
  })

  it('does not record a late navigation into the next connection history', async () => {
    const files = useFilesStore()
    const pending = deferred<FileInfo[]>()
    mockApi.get.mockReturnValue(pending.promise)
    const navigation = files.navigate('/')
    files.$reset()
    pending.resolve([])
    expect(await navigation).toBe(false)
    mockApi.get.mockResolvedValue([])
    await files.navigate('/new')
    expect(files.canGoBack).toBe(false)
  })

  it.each(['token', 'disconnect', 'lost'] as const)('clears snapshots and expansion on %s changes and rejects late responses', async (change) => {
    const auth = useAuthStore()
    auth.connected = true
    auth.csrfToken = 'first'
    const store = useDirectoriesStore()
    const files = useFilesStore()
    const pending = deferred<FileInfo[]>()
    mockApi.get.mockReturnValue(pending.promise)
    const listing = store.loadTree('/old').catch(e => e)
    const navigation = files.list('/old')
    store.expanded = ['/', '/old']
    if (change === 'token')
      auth.csrfToken = 'second'
    else if (change === 'disconnect')
      auth.connected = false
    else
      auth.sessionLost = true
    pending.resolve([directory('old')])
    const error = await listing
    await navigation
    expect(error.code).toBe(ERR_ABORTED)
    expect(store.listings).toEqual({})
    expect(store.expanded).toEqual(['/'])
    expect(files.currentPath).toBe('/')
    expect(files.files).toEqual([])
  })

  it('invalidates source and destination trees for a cross-directory move', async () => {
    const store = useDirectoriesStore()
    for (const path of ['/', '/src', '/src/folder/nested', '/dst', '/unrelated'])
      await store.load(path)
    await useFilesStore().move('/src/folder', '/dst/folder')
    for (const path of ['/', '/src', '/src/folder/nested', '/dst'])
      expect(store.listings[path]?.dirty).toBe(true)
    expect(store.listings['/unrelated']?.dirty).toBe(false)
  })

  it('invalidates failed recursive copies because they can leave partial directories', async () => {
    const store = useDirectoriesStore()
    await store.load('/dst')
    mockApi.patch.mockRejectedValue(new ApiError('ERR_FILE_PERMISSION', 'denied'))
    await expect(useFilesStore().copy('/src', '/dst/copy')).rejects.toThrow('denied')
    expect(store.listings['/dst']?.dirty).toBe(true)
  })

  it('refreshes partial deletions and invalidates descendants without deleting failed entries from the cache', async () => {
    const store = useDirectoriesStore()
    await store.load('/home/gone/child')
    await store.load('/home/private/child')
    const files = useFilesStore()
    await files.list('/home')
    mockApi.del.mockResolvedValue({ deleted: ['/home/gone'], failed: [{ path: '/home/private', code: 'ERR_FILE_PERMISSION', message: 'denied' }] })
    mockApi.get.mockResolvedValue([directory('private')])
    await files.deleteFiles(['/home/gone', '/home/private'])
    expect(store.listings['/home']?.files).toEqual([directory('private')])
    expect(store.listings['/home/gone/child']?.dirty).toBe(true)
    expect(store.listings['/home/private/child']?.dirty).toBe(true)
  })

  it('invalidates all cached ancestors when an upload creates nested folders', async () => {
    const store = useDirectoriesStore()
    for (const path of ['/', '/upload', '/upload/a'])
      await store.load(path)
    await useFilesStore().ensureDir('/upload/a/b/c')
    for (const path of ['/', '/upload', '/upload/a'])
      expect(store.listings[path]?.dirty).toBe(true)
  })
})
