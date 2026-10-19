import type { DirectorySizeResult } from '~/types/api'
import { defineStore } from 'pinia'
import { ApiError, ERR_ABORTED, ERR_UNKNOWN } from '~/types/api'

export type DirectorySizeStatus = 'unknown' | 'loading' | 'complete' | 'partial' | 'error'

export interface DirectorySizeEntry {
  status: DirectorySizeStatus
  result?: DirectorySizeResult
  errorCode?: string
}

const UNKNOWN: DirectorySizeEntry = { status: 'unknown' }

export function directorySizePath(directory: string, name?: string): string {
  const path = name === undefined ? directory : `${directory.replace(/\/$/, '')}/${name}`
  return path.replace(/\/+$/, '') || '/'
}

export const useDirectorySizesStore = defineStore('directorySizes', () => {
  const entries = ref<Record<string, DirectorySizeEntry>>({})
  const activePath = ref<string | null>(null)
  const connectionGeneration = ref(0)
  let generation = 0
  let controller: AbortController | null = null
  let pending: Promise<void> | null = null

  function get(path: string): DirectorySizeEntry {
    return entries.value[directorySizePath(path)] ?? UNKNOWN
  }

  function restore(path: string, result?: DirectorySizeResult) {
    if (result)
      entries.value[path] = { status: result.complete ? 'complete' : 'partial', result }
    else
      delete entries.value[path]
  }

  function cancel() {
    generation++
    controller?.abort()
    if (activePath.value && entries.value[activePath.value]?.status === 'loading')
      restore(activePath.value, entries.value[activePath.value]?.result)
    activePath.value = null
    controller = null
    pending = null
  }

  function invalidate(connection = connectionGeneration.value) {
    if (connection !== connectionGeneration.value)
      return
    cancel()
    entries.value = {}
  }

  function $reset() {
    connectionGeneration.value++
    invalidate()
  }

  function calculate(rawPath: string): Promise<void> {
    const path = directorySizePath(rawPath)
    if (activePath.value === path && pending)
      return pending
    cancel()
    const previous = get(path).result
    const requestGeneration = generation
    const abort = new AbortController()
    controller = abort
    activePath.value = path
    entries.value[path] = { status: 'loading', result: previous }

    pending = (async () => {
      try {
        const result = await useApi().post<DirectorySizeResult>('/api/files/directory-size', { path }, { signal: abort.signal })
        if (requestGeneration !== generation || abort.signal.aborted)
          return
        entries.value[path] = {
          status: result.complete ? 'complete' : 'partial',
          result,
        }
      }
      catch (error) {
        if (requestGeneration !== generation || abort.signal.aborted)
          return
        if (error instanceof ApiError && error.code === ERR_ABORTED)
          restore(path, previous)
        else
          entries.value[path] = { status: 'error', result: previous, errorCode: error instanceof ApiError ? error.code : ERR_UNKNOWN }
      }
      finally {
        if (requestGeneration === generation) {
          activePath.value = null
          controller = null
          pending = null
        }
      }
    })()
    return pending
  }

  return { entries, activePath, connectionGeneration, get, calculate, cancel, invalidate, $reset }
})
