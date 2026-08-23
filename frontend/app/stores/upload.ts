import type { UploadProgress } from '~/composables/useApi'
import type { UploadConflictAction, UploadConflictResolution } from '~/stores/modal'
import type { UploadConflict } from '~/types/api'
import type { RateSample } from '~/utils/transferRate'
import { defineStore } from 'pinia'
import { ApiError } from '~/types/api'

export type UploadStatus = 'checking' | 'queued' | 'uploading' | 'done' | 'error' | 'cancelled' | 'skipped'

export interface UploadItem {
  id: string
  file: File
  destPath: string
  // Path shown in the queue, relative to the drop target (e.g. "folder/sub/a.txt").
  // For loose files this is just the file name.
  relativePath?: string
  status: UploadStatus
  progress: number
  bytesUploaded: number
  // The bytes are all sent but the server is still writing them to the remote,
  // which we cannot observe. Distinct from a status so it does not ripple into
  // STATUS_CLASS, the cancellable sets, the aggregate filter and 13 locales.
  finalizing?: boolean
  error?: string
  errorCode?: string
  // The user agreed to replace an existing file, so the backend guard is waived.
  overwrite?: boolean
  // Set once chunks are staged. Keeping it lets a conflict at commit time be
  // resolved by re-committing rather than re-uploading the whole file.
  uploadId?: string
  // A conflict raised mid-transfer is re-prompted once; a second one is an error
  // rather than another prompt, so a racing server cannot loop the dialog.
  reprompted?: boolean
}

const MAX_RETRIES = 5

// Retrying these is pointless - nothing about the request will change. Without
// ERR_FILE_EXISTS here a conflict would burn ~31s of backoff before surfacing.
// Errors a retry can never recover from. Includes the session-lost family:
// retrying those burned 5 attempts and ~31s of backoff per queued item after a
// dropped connection, with markSessionLost() firing on each one.
const NON_RETRYABLE = new Set([
  'ERR_FILE_EXISTS',
  'ERR_FILE_PERMISSION',
  'ERR_QUOTA_EXCEEDED',
  'ERR_SESSION_NOT_FOUND',
  'ERR_UNAUTHORIZED',
  'ERR_CSRF_INVALID',
  'ERR_CONNECTION_LOST',
  'ERR_UPLOAD_NOT_FOUND',
])

export const useUploadStore = defineStore('upload', () => {
  const items = ref<UploadItem[]>([])
  const _active = ref(0)
  // "Apply to all" from the batch dialog, reused for conflicts that only appear
  // once transfers are under way.
  const _blanketPolicy = ref<UploadConflictAction | null>(null)
  // Serializes conflict dialogs; the modal store holds one resolver at a time.
  let _conflictChain: Promise<unknown> = Promise.resolve()

  const authStore = useAuthStore()
  const filesStore = useFilesStore()

  const chunkSize = computed(() => authStore.systemVars?.upload.chunkSize ?? 5 * 1024 * 1024)
  // The backend serializes all transfers on a session's single control connection
  // (per-session transfer lock) and guards its session state with a mutex, so any
  // value here is safe. Default 1 because one FTP/SFTP connection transfers one
  // file at a time; GFTP_UPLOAD_MAX_CONCURRENT can raise it, though uploads then
  // queue on the backend transfer lock rather than truly running in parallel.
  const maxConcurrent = computed(() => authStore.systemVars?.upload.maxConcurrentUploads ?? 1)

  const hasActive = computed(() => items.value.some(
    i => i.status === 'checking' || i.status === 'queued' || i.status === 'uploading',
  ))

  // ── Throughput sampling ────────────────────────────────────────────────────
  // Per-item bytes/second, or null while still measuring. Zero means stalled.
  const rates = ref<Record<string, number | null>>({})
  // Windows are plain (non-reactive) state: they change twice a second per item
  // and nothing renders them directly, only the derived rate.
  const windows = new Map<string, RateSample[]>()
  // Abort handles for in-flight requests, keyed by item id. Deliberately not on
  // the reactive item: nothing renders them, and a controller in a ref would be
  // proxied for no reason.
  const _controllers = new Map<string, AbortController>()

  // Returns a fresh signal for this item's next request, replacing any handle
  // from a previous attempt. A retry gets a new controller, so an abort of the
  // old attempt cannot cancel the new one.
  function _signalFor(item: UploadItem): AbortSignal {
    const controller = new AbortController()
    _controllers.set(item.id, controller)
    return controller.signal
  }

  // xhr.upload.onprogress fires every few milliseconds on a fast link. Writing
  // through to the store that often thrashes reactivity across the whole queue
  // for detail no one can see.
  function _throttledProgress(fn: (p: UploadProgress) => void) {
    return useThrottleFn(fn, 100, true)
  }

  function _sampleTick() {
    const at = Date.now()
    const active = items.value.filter(i => i.status === 'uploading')
    for (const item of active) {
      const next = pushSample(windows.get(item.id) ?? [], { at, bytes: item.bytesUploaded })
      windows.set(item.id, next)
      rates.value[item.id] = rateFromSamples(next)
    }
    // Drop windows for items that are no longer uploading, so a queue that runs
    // all day does not accumulate them.
    const live = new Set(active.map(i => i.id))
    for (const id of windows.keys()) {
      if (!live.has(id)) {
        windows.delete(id)
        delete rates.value[id]
      }
    }
  }

  useIntervalFn(_sampleTick, SAMPLE_INTERVAL_MS)

  // Batch throughput: the sum of the per-item rates currently known. Only
  // meaningful while something is uploading.
  const overallRate = computed(() => {
    const values = items.value
      .filter(i => i.status === 'uploading' && !i.finalizing)
      .map(i => rates.value[i.id])
      .filter((r): r is number => typeof r === 'number')
    if (values.length === 0)
      return null
    return values.reduce((a, b) => a + b, 0)
  })

  const overallEtaSeconds = computed(() => {
    const pending = items.value.filter(i => i.status === 'uploading' || i.status === 'queued')
    const remaining = pending.reduce((sum, i) => sum + Math.max(0, i.file.size - i.bytesUploaded), 0)
    return etaSeconds(remaining, overallRate.value)
  })

  // Refresh the listing once a burst of completions settles, instead of once per
  // file - a folder upload otherwise fires a refresh storm (one list() per file).
  const scheduleRefresh = useDebounceFn(() => filesStore.list(), 400)

  // Enqueue files carrying their nested relative paths (from a folder drop).
  // destPath preserves the structure; the backend creates missing parent dirs.
  //
  // Nothing is sent until the pre-flight has reported which destinations are
  // already occupied and the user has said what to do about them.
  async function addEntries(entries: { file: File, relativePath: string }[], destDir: string) {
    const base = destDir.replace(/\/$/, '')
    const newItems: UploadItem[] = entries.map(({ file, relativePath }) => ({
      id: uid(),
      file,
      destPath: `${base}/${relativePath}`,
      relativePath,
      status: 'checking' as UploadStatus,
      progress: 0,
      bytesUploaded: 0,
    }))
    if (newItems.length === 0)
      return
    // Added as 'checking' first so the queue panel shows the batch while a slow
    // pre-flight runs.
    const ids = new Set(newItems.map(i => i.id))
    items.value = [...items.value, ...newItems]

    // Always go back through items.value: newItems holds the raw objects, and
    // mutating those bypasses the reactive proxy, so the queue would keep
    // rendering a stale status.
    const mine = () => items.value.filter(i => ids.has(i.id))
    const live = () => mine().filter(i => i.status === 'checking')

    const conflicts = await _checkConflicts(mine().map(i => i.destPath))

    if (conflicts.length > 0) {
      const resolution = await useModalStore().uploadConflict(conflicts)
      if (resolution.kind === 'cancel') {
        live().forEach((i) => {
          i.status = 'cancelled'
        })
        return
      }
      _blanketPolicy.value = resolution.applyToAll ?? null
      const byPath = new Map(conflicts.map(c => [c.path, c]))
      for (const item of live()) {
        const conflict = byPath.get(item.destPath)
        if (!conflict)
          continue
        _applyDecision(item, resolution.decisions[item.destPath] ?? 'rename', conflict.suggestedName)
      }
    }

    live().forEach((i) => {
      i.status = 'queued'
    })
    _processQueue()
  }

  async function addFiles(files: FileList | File[], destDir: string) {
    await addEntries(Array.from(files).map(file => ({ file, relativePath: file.name })), destDir)
  }

  // A pre-flight failure must never block the upload: the per-request guard on
  // the backend still raises conflicts, they just surface one file at a time.
  async function _checkConflicts(paths: string[]): Promise<UploadConflict[]> {
    try {
      const res = await useApi().post<{ conflicts: UploadConflict[] }>('/api/files/upload/check', { paths })
      return res.conflicts ?? []
    }
    catch {
      return []
    }
  }

  function _applyDecision(item: UploadItem, action: UploadConflictAction, suggestedName: string) {
    if (action === 'overwrite') {
      item.overwrite = true
      return
    }
    if (action === 'skip') {
      item.status = 'skipped'
      return
    }
    item.destPath = _withName(item.destPath, suggestedName)
    if (item.relativePath)
      item.relativePath = _withName(item.relativePath, suggestedName)
  }

  function _withName(path: string, name: string): string {
    const slash = path.lastIndexOf('/')
    return slash < 0 ? name : `${path.slice(0, slash)}/${name}`
  }

  function _processQueue() {
    while (_active.value < maxConcurrent.value) {
      const next = items.value.find(i => i.status === 'queued')
      if (!next)
        break
      next.status = 'uploading'
      _active.value++
      _runUpload(next).finally(() => {
        _active.value--
        if (next.status === 'done')
          scheduleRefresh()
        // The batch is over, so its "apply to all" should not leak into the next.
        if (!hasActive.value)
          _blanketPolicy.value = null
        _processQueue()
      })
    }
  }

  async function _runUpload(item: UploadItem) {
    try {
      await _transfer(item)
      // Guard: if cancelled mid-request, don't overwrite 'cancelled' with 'done'
      if (item.status !== 'cancelled') {
        item.progress = 100
        item.bytesUploaded = item.file.size
        item.status = 'done'
      }
    }
    catch (e) {
      if (item.status === 'cancelled' || item.status === 'skipped')
        return
      item.status = 'error'
      item.errorCode = e instanceof ApiError ? e.code : undefined
      item.error = e instanceof Error ? e.message : 'Upload failed'
      // Nothing sweeps the staging area, so a failed chunked upload has to
      // release its chunks here or they occupy the server volume forever.
      // This also clears uploadId, which is what makes a later retry re-send
      // the file instead of committing a half-staged set.
      await _discardStaged(item)
    }
  }

  async function _transfer(item: UploadItem) {
    try {
      if (item.file.size <= chunkSize.value)
        await _uploadSimple(item)
      else
        await _uploadChunked(item)
    }
    catch (e) {
      // The destination was free at pre-flight but is occupied now: re-decide
      // rather than failing, since the user never saw this conflict.
      if (e instanceof ApiError && e.code === 'ERR_FILE_EXISTS' && !item.reprompted) {
        item.reprompted = true
        await _resolveRaced(item)
        if (item.status === 'skipped')
          return
        await _transfer(item)
        return
      }
      throw e
    }
  }

  // Applies the batch-wide choice if there is one, otherwise asks. Prompts are
  // serialized because the modal store keeps a single resolver - two concurrent
  // conflicts would otherwise cancel each other's dialog.
  async function _resolveRaced(item: UploadItem) {
    const [conflict] = await _checkConflicts([item.destPath])
    // Vanished again - just retry as-is.
    if (!conflict)
      return

    let action = _blanketPolicy.value
    if (!action) {
      _conflictChain = _conflictChain.then(() => useModalStore().uploadConflict([conflict]))
      const resolution = await (_conflictChain as Promise<UploadConflictResolution>)
      if (resolution.kind === 'cancel') {
        item.status = 'cancelled'
        await _discardStaged(item)
        return
      }
      _blanketPolicy.value = resolution.applyToAll ?? null
      action = resolution.decisions[conflict.path] ?? 'rename'
    }

    _applyDecision(item, action, conflict.suggestedName)
    if (item.status === 'skipped')
      await _discardStaged(item)
  }

  // Nothing sweeps the staging area, so an abandoned chunked upload must be
  // released explicitly or its chunks live until the disk fills.
  async function _discardStaged(item: UploadItem) {
    if (!item.uploadId)
      return
    const uploadId = item.uploadId
    item.uploadId = undefined
    try {
      await useApi().post('/api/files/upload/abort', { uploadId })
    }
    catch {
      // Best effort: a failed abort must not mask the user's decision.
    }
  }

  async function _uploadSimple(item: UploadItem) {
    await _withRetry(async () => {
      if (item.status === 'cancelled')
        throw new Error('Cancelled')
      const api = useApi()
      const form = new FormData()
      form.append('path', item.destPath)
      form.append('file', item.file, item.file.name)
      if (item.overwrite)
        form.append('overwrite', 'true')
      try {
        await api.postForm('/api/files/upload', form, {
          signal: _signalFor(item),
          // total is the multipart body size, a little larger than the file, so
          // clamp: the queue row renders bytesUploaded against file.size and
          // would otherwise read "1.1 MiB / 1.0 MiB".
          onProgress: _throttledProgress(({ loaded, total }) => {
            item.bytesUploaded = Math.min(item.file.size, loaded)
            item.progress = total > 0 ? Math.round((loaded / total) * 100) : 0
            if (loaded >= total)
              item.finalizing = true
          }),
        })
      }
      finally {
        item.finalizing = false
      }
    }, item)
  }

  async function _uploadChunked(item: UploadItem) {
    const api = useApi()

    // Already staged (a conflict was resolved after the bytes went up): commit
    // the existing chunks instead of sending the file again.
    if (!item.uploadId) {
      const totalChunks = Math.ceil(item.file.size / chunkSize.value)
      const { uploadId } = await api.post<{ uploadId: string }>('/api/files/upload/reserve', {
        path: item.destPath,
        totalChunks,
        totalSize: item.file.size,
        chunkSize: chunkSize.value,
        overwrite: item.overwrite ?? false,
      })
      item.uploadId = uploadId

      for (let i = 0; i < totalChunks; i++) {
        if (item.status === 'cancelled')
          throw new Error('Cancelled')

        const start = i * chunkSize.value
        const end = Math.min(start + chunkSize.value, item.file.size)
        const chunk = item.file.slice(start, end)

        await _withRetry(async () => {
          if (item.status === 'cancelled')
            throw new Error('Cancelled')
          const form = new FormData()
          form.append('uploadId', uploadId)
          form.append('chunkIndex', String(i))
          form.append('chunk', chunk, item.file.name)
          await api.postForm('/api/files/upload/chunk', form, {
            signal: _signalFor(item),
            onProgress: _throttledProgress(({ loaded, total }) => {
              const sent = total > 0 ? Math.min(chunk.size, (loaded / total) * chunk.size) : 0
              item.bytesUploaded = Math.min(item.file.size, start + sent)
              item.progress = Math.round((item.bytesUploaded / item.file.size) * 100)
            }),
          })
        }, item)

        // Authoritative settle: a retried chunk replays onProgress from zero,
        // so the running figure can dip by up to one chunk before this lands.
        item.bytesUploaded = end
        item.progress = Math.round((end / item.file.size) * 100)
      }
    }

    // Commit is where the backend actually pushes the staged bytes to the FTP/
    // SFTP server, so the bar would otherwise read 100% for the entire real
    // transfer.
    item.finalizing = true
    try {
      // destination carries a rename decided after the reserve; the backend keeps
      // it inside the reserved directory.
      await api.post('/api/files/upload/commit', {
        uploadId: item.uploadId,
        overwrite: item.overwrite ?? false,
        destination: item.destPath,
      })
    }
    finally {
      item.finalizing = false
    }
    item.uploadId = undefined
  }

  async function _withRetry<T>(fn: () => Promise<T>, item?: UploadItem): Promise<T> {
    let lastError: unknown
    for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
      try {
        return await fn()
      }
      catch (e) {
        if (e instanceof Error && e.message === 'Cancelled')
          throw e
        if (e instanceof ApiError && e.code === 'ERR_ABORTED')
          throw e
        if (e instanceof ApiError && NON_RETRYABLE.has(e.code))
          throw e
        lastError = e
        // The backoff has to be interruptible: sleeping it out kept _active
        // incremented for up to 31s after Cancel, blocking the whole queue on
        // an item the user had already given up on.
        await _backoff(Math.min(30_000, 1000 * 2 ** attempt), item)
        if (item && (item.status === 'cancelled' || item.status === 'skipped'))
          throw new Error('Cancelled')
      }
    }
    throw lastError
  }

  // Sleeps ms, returning early once the item is no longer running.
  function _backoff(ms: number, item?: UploadItem): Promise<void> {
    return new Promise((resolve) => {
      const started = Date.now()
      const tick = window.setInterval(() => {
        const stopped = item && (item.status === 'cancelled' || item.status === 'skipped')
        if (stopped || Date.now() - started >= ms) {
          window.clearInterval(tick)
          resolve()
        }
      }, 200)
    })
  }

  const CANCELLABLE: UploadStatus[] = ['checking', 'queued', 'uploading']

  function _abort(item: UploadItem) {
    // Aborting the in-flight XHR is what actually stops the bytes; setting the
    // status alone only relabelled the row while the upload ran to completion.
    _controllers.get(item.id)?.abort()
    _controllers.delete(item.id)
  }

  function cancelItem(id: string) {
    const item = items.value.find(i => i.id === id)
    if (item && CANCELLABLE.includes(item.status)) {
      item.status = 'cancelled'
      _abort(item)
      void _discardStaged(item)
    }
  }

  function cancelAll() {
    items.value.forEach((item) => {
      if (CANCELLABLE.includes(item.status)) {
        item.status = 'cancelled'
        _abort(item)
        void _discardStaged(item)
      }
    })
  }

  // Re-queue a failed, cancelled or skipped item; _runUpload re-runs it from
  // scratch. A skipped item re-runs without consent, so it conflicts again and
  // re-prompts rather than silently overwriting.
  async function retryItem(id: string) {
    const item = items.value.find(i => i.id === id)
    if (item && (item.status === 'error' || item.status === 'cancelled' || item.status === 'skipped')) {
      // Release any chunks still staged from the abandoned attempt before
      // re-queueing. Leaving uploadId set made _uploadChunked take its
      // "already staged, just commit" branch, so a retry skipped every chunk
      // and committed a partial set - which could never succeed.
      await _discardStaged(item)
      item.status = 'queued'
      item.progress = 0
      item.bytesUploaded = 0
      item.finalizing = false
      item.error = undefined
      item.errorCode = undefined
      item.reprompted = false
      // The old window describes the abandoned attempt; pushSample would also
      // discard it on the byte rewind, but clearing here avoids showing a stale
      // rate in the meantime.
      windows.delete(id)
      delete rates.value[id]
      _processQueue()
    }
  }

  function clearDone() {
    // Fire-and-forget: dropping an errored item from the list must not leave
    // its chunks stranded on the server.
    for (const item of items.value) {
      if (item.uploadId && item.status !== 'uploading')
        void _discardStaged(item)
    }
    items.value = items.value.filter(
      i => i.status !== 'done' && i.status !== 'error' && i.status !== 'cancelled' && i.status !== 'skipped',
    )
  }

  function $reset() {
    // Release staged chunks before dropping the items that reference them.
    // Best-effort: a disconnect must not block on the server answering.
    for (const item of items.value) {
      if (item.uploadId)
        void _discardStaged(item)
    }
    items.value = []
    _active.value = 0
    _blanketPolicy.value = null
    windows.clear()
    rates.value = {}
  }

  return {
    items,
    hasActive,
    rates,
    overallRate,
    overallEtaSeconds,
    chunkSize,
    maxConcurrent,
    addFiles,
    addEntries,
    cancelItem,
    cancelAll,
    retryItem,
    clearDone,
    $reset,
  }
})
