import { defineStore } from 'pinia'

export type UploadStatus = 'queued' | 'uploading' | 'done' | 'error' | 'cancelled'

export interface UploadItem {
  id: string
  file: File
  destPath: string
  status: UploadStatus
  progress: number
  bytesUploaded: number
  error?: string
}

const MAX_RETRIES = 5

export const useUploadStore = defineStore('upload', () => {
  const items = ref<UploadItem[]>([])
  const _active = ref(0)

  const authStore = useAuthStore()
  const filesStore = useFilesStore()

  const chunkSize = computed(() => authStore.systemVars?.upload.chunkSize ?? 5 * 1024 * 1024)
  // The backend serializes all transfers on a session's single control connection
  // (per-session transfer lock) and guards its session state with a mutex, so any
  // value here is safe. Default 1 because one FTP/SFTP connection transfers one
  // file at a time; GFTP_MAX_CONCURRENT_UPLOADS can raise it, though uploads then
  // queue on the backend transfer lock rather than truly running in parallel.
  const maxConcurrent = computed(() => authStore.systemVars?.upload.maxConcurrentUploads ?? 1)

  const hasActive = computed(() => items.value.some(i => i.status === 'queued' || i.status === 'uploading'))

  function addFiles(files: FileList | File[], destDir: string) {
    const normalized = destDir.replace(/\/$/, '')
    const newItems: UploadItem[] = Array.from(files).map(file => ({
      id: crypto.randomUUID(),
      file,
      destPath: `${normalized}/${file.name}`,
      status: 'queued' as UploadStatus,
      progress: 0,
      bytesUploaded: 0,
    }))
    items.value = [...items.value, ...newItems]
    _processQueue()
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
          filesStore.list()
        _processQueue()
      })
    }
  }

  async function _runUpload(item: UploadItem) {
    try {
      if (item.file.size <= chunkSize.value) {
        await _uploadSimple(item)
      }
      else {
        await _uploadChunked(item)
      }
      // Guard: if cancelled mid-request, don't overwrite 'cancelled' with 'done'
      if (item.status !== 'cancelled') {
        item.progress = 100
        item.bytesUploaded = item.file.size
        item.status = 'done'
      }
    }
    catch (e) {
      if (item.status !== 'cancelled') {
        item.status = 'error'
        item.error = e instanceof Error ? e.message : 'Upload failed'
      }
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
      await api.post('/api/files/upload', form)
    })
  }

  async function _uploadChunked(item: UploadItem) {
    const api = useApi()
    const totalChunks = Math.ceil(item.file.size / chunkSize.value)
    const { uploadId } = await api.post<{ uploadId: string }>('/api/files/upload/reserve', {
      path: item.destPath,
      totalChunks,
      totalSize: item.file.size,
      chunkSize: chunkSize.value,
    })

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
        await api.post('/api/files/upload/chunk', form)
      })

      item.bytesUploaded = end
      item.progress = Math.round((end / item.file.size) * 100)
    }

    await api.post('/api/files/upload/commit', { uploadId })
  }

  async function _withRetry<T>(fn: () => Promise<T>): Promise<T> {
    let lastError: unknown
    for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
      try {
        return await fn()
      }
      catch (e) {
        if (e instanceof Error && e.message === 'Cancelled')
          throw e
        lastError = e
        await new Promise(r => setTimeout(r, Math.min(30_000, 1000 * 2 ** attempt)))
      }
    }
    throw lastError
  }

  function cancelItem(id: string) {
    const item = items.value.find(i => i.id === id)
    if (item && (item.status === 'queued' || item.status === 'uploading'))
      item.status = 'cancelled'
  }

  function cancelAll() {
    items.value.forEach((item) => {
      if (item.status === 'queued' || item.status === 'uploading')
        item.status = 'cancelled'
    })
  }

  function clearDone() {
    items.value = items.value.filter(
      i => i.status !== 'done' && i.status !== 'error' && i.status !== 'cancelled',
    )
  }

  function $reset() {
    items.value = []
    _active.value = 0
  }

  return {
    items,
    hasActive,
    chunkSize,
    maxConcurrent,
    addFiles,
    cancelItem,
    cancelAll,
    clearDone,
    $reset,
  }
})
