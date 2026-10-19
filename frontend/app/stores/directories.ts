import type { FileInfo } from '~/types/api'
import { defineStore } from 'pinia'
import { ApiError, ERR_ABORTED } from '~/types/api'
import { ancestorPaths, parentPath, pathContains, remotePath } from '~/utils/remotePath'

export interface DirectoryListing {
  files: FileInfo[] | null
  loading: boolean
  dirty: boolean
  errorCode: string | null
  revision: number
}

export const useDirectoriesStore = defineStore('directories', () => {
  const auth = useAuthStore()
  const listings = ref<Record<string, DirectoryListing>>({})
  const expanded = ref<string[]>(['/'])
  const invalidation = ref(0)
  let generation = 0
  let pending = new Map<string, Promise<FileInfo[]>>()
  let queued = new Map<string, Promise<FileInfo[]>>()
  let treeQueue: Promise<unknown> = Promise.resolve()

  function entry(path: string): DirectoryListing {
    path = remotePath(path)
    listings.value[path] ??= { files: null, loading: false, dirty: true, errorCode: null, revision: 0 }
    return listings.value[path]!
  }

  function load(path: string, fresh = false): Promise<FileInfo[]> {
    path = remotePath(path)
    const existing = pending.get(path)
    if (existing)
      return existing.then(files => fresh && entry(path).dirty ? load(path, true) : files)
    const state = entry(path)
    if (!fresh && state.files !== null && !state.dirty)
      return Promise.resolve(state.files)
    const epoch = generation
    const revision = state.revision
    state.loading = true
    state.errorCode = null
    const request = (async () => {
      try {
        const files = await useApi().get<FileInfo[]>(`/api/files?path=${encodeURIComponent(path)}`)
        if (epoch !== generation)
          throw new ApiError(ERR_ABORTED, 'Connection changed')
        state.files = files
        state.dirty = state.revision !== revision
        return files
      }
      catch (e) {
        if (epoch === generation) {
          state.dirty = true
          state.errorCode = e instanceof ApiError ? e.code : 'ERR_UNKNOWN'
        }
        throw e
      }
      finally {
        if (epoch === generation) {
          state.loading = false
          pending.delete(path)
        }
      }
    })()
    pending.set(path, request)
    return request
  }

  function loadTree(path: string): Promise<FileInfo[]> {
    path = remotePath(path)
    const existing = pending.get(path) ?? queued.get(path)
    if (existing)
      return existing
    const epoch = generation
    const request = treeQueue.then(() => {
      if (epoch !== generation)
        throw new ApiError(ERR_ABORTED, 'Connection changed')
      return load(path)
    }).finally(() => {
      if (epoch === generation)
        queued.delete(path)
    })
    queued.set(path, request)
    treeQueue = request.catch(() => {})
    return request
  }

  // Invalidate ancestors too: mkdir -p and recursive copies can create several levels.
  function invalidate(paths: string[]) {
    const normalized = paths.map(remotePath)
    const parents = new Set(normalized.flatMap(path => ancestorPaths(parentPath(path))))
    for (const [path, state] of Object.entries(listings.value)) {
      if (parents.has(path) || normalized.some(changed => pathContains(changed, path))) {
        state.dirty = true
        state.revision++
        state.errorCode = null
      }
    }
    invalidation.value++
  }

  function $reset() {
    generation++
    listings.value = {}
    expanded.value = ['/']
    pending = new Map()
    queued = new Map()
    treeQueue = Promise.resolve()
  }

  watch(() => [auth.csrfToken, auth.connected, auth.sessionLost], $reset, { flush: 'sync' })

  return { listings, expanded, invalidation, entry, load, loadTree, invalidate, $reset }
})
