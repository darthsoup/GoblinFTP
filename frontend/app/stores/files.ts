import type { PasteChoice } from '~/stores/modal'
import type { FileInfo } from '~/types/api'
import { defineStore } from 'pinia'
import { ApiError } from '~/types/api'

export interface ClipboardState {
  mode: 'copy' | 'cut'
  sourcePath: string // directory the items were copied/cut from (no trailing slash)
  names: string[]
}

export interface PasteResult {
  mode: 'copy' | 'cut'
  ok: number
  failed: number
  cancelled?: boolean
}

// Returns a name not present in `existing`, suffixing " (copy)" / " (copy N)"
// before the extension — like a desktop file manager.
function uniqueName(name: string, existing: Set<string>): string {
  if (!existing.has(name))
    return name
  const dot = name.lastIndexOf('.')
  const base = dot > 0 ? name.slice(0, dot) : name
  const ext = dot > 0 ? name.slice(dot) : ''
  let candidate = `${base} (copy)${ext}`
  let i = 2
  while (existing.has(candidate)) {
    candidate = `${base} (copy ${i})${ext}`
    i++
  }
  return candidate
}

export const useFilesStore = defineStore('files', () => {
  const currentPath = ref('/')
  const files = ref<FileInfo[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const selected = ref<Set<string>>(new Set())
  // Name of the file currently being renamed in place (null = none)
  const editingName = ref<string | null>(null)
  // Copy/cut clipboard (null = empty). Paste targets the current directory.
  const clipboard = ref<ClipboardState | null>(null)

  // Navigation history (back/forward) — only `navigate()` pushes entries
  const history = ref<string[]>([])
  const historyIndex = ref(-1)
  const canGoBack = computed(() => historyIndex.value > 0)
  const canGoForward = computed(() => historyIndex.value >= 0 && historyIndex.value < history.value.length - 1)

  async function list(path?: string) {
    const api = useApi()
    const target = path ?? currentPath.value
    loading.value = true
    error.value = null
    editingName.value = null
    try {
      const result = await api.get<FileInfo[]>(`/api/files?path=${encodeURIComponent(target)}`)
      files.value = result
      currentPath.value = target
      selected.value = new Set()
      // Seed history with the first successfully listed directory
      if (history.value.length === 0) {
        history.value = [target]
        historyIndex.value = 0
      }
    }
    catch (e) {
      error.value = e instanceof ApiError ? e.message : 'Failed to list directory'
    }
    finally {
      loading.value = false
    }
  }

  async function navigate(path: string) {
    await list(path)
    // Record only successful navigations; skip refreshes of the current entry
    if (error.value || currentPath.value !== path)
      return
    if (history.value[historyIndex.value] === path)
      return
    history.value = [...history.value.slice(0, historyIndex.value + 1), path]
    historyIndex.value = history.value.length - 1
  }

  async function goBack() {
    if (!canGoBack.value)
      return
    const target = history.value[historyIndex.value - 1]!
    await list(target)
    if (!error.value && currentPath.value === target)
      historyIndex.value--
  }

  async function goForward() {
    if (!canGoForward.value)
      return
    const target = history.value[historyIndex.value + 1]!
    await list(target)
    if (!error.value && currentPath.value === target)
      historyIndex.value++
  }

  async function navigateUp() {
    const parts = currentPath.value.split('/').filter(Boolean)
    parts.pop()
    const parent = parts.length > 0 ? `/${parts.join('/')}` : '/'
    await navigate(parent)
  }

  // Build an authenticated, short-lived download URL (token in the query string).
  async function downloadUrl(path: string): Promise<string> {
    const api = useApi()
    const data = await api.post<{ token: string }>('/api/files/download-token', { path })
    return `/api/files/download?path=${encodeURIComponent(path)}&token=${data.token}`
  }

  async function downloadFile(filePath: string): Promise<void> {
    window.open(await downloadUrl(filePath), '_blank')
  }

  // Fetch a file as a typed object URL for inline preview. The download endpoint
  // serves application/octet-stream, so we re-wrap the bytes with the real MIME
  // type. The caller owns the returned URL and must URL.revokeObjectURL() it.
  async function fetchObjectUrl(path: string, mime: string): Promise<string> {
    const url = await downloadUrl(path)
    const resp = await $fetch.raw(url, { responseType: 'blob' })
    const blob = new Blob([resp._data as Blob], { type: mime })
    return URL.createObjectURL(blob)
  }

  function toggleSelection(name: string) {
    const next = new Set(selected.value)
    if (next.has(name))
      next.delete(name)
    else next.add(name)
    selected.value = next
  }

  function clearSelection() {
    selected.value = new Set()
  }

  function setSelection(names: string[]) {
    selected.value = new Set(names)
  }

  function startRename(name: string) {
    editingName.value = name
  }

  function cancelRename() {
    editingName.value = null
  }

  async function rename(from: string, to: string): Promise<void> {
    const api = useApi()
    await api.patch('/api/files/rename', { from, to })
    await list()
  }

  // ── Clipboard (copy / cut → paste) ──────────────────────────────────────────
  function copyToClipboard(names: string[]) {
    if (names.length === 0)
      return
    clipboard.value = { mode: 'copy', sourcePath: currentPath.value.replace(/\/$/, ''), names: [...names] }
  }

  function cutToClipboard(names: string[]) {
    if (names.length === 0)
      return
    clipboard.value = { mode: 'cut', sourcePath: currentPath.value.replace(/\/$/, ''), names: [...names] }
  }

  function clearClipboard() {
    clipboard.value = null
  }

  async function copy(from: string, to: string): Promise<void> {
    const api = useApi()
    await api.patch('/api/files/copy', { from, to })
  }

  // Move reuses the rename endpoint — native Rename is a cross-directory move on
  // both FTP and SFTP (no separate backend endpoint needed). No list refresh here.
  async function move(from: string, to: string): Promise<void> {
    const api = useApi()
    await api.patch('/api/files/rename', { from, to })
  }

  // Paste the clipboard into the current directory. On name collisions it asks
  // the user (overwrite / append / cancel) via the modal store, then applies the
  // choice to the whole batch. Copy keeps the clipboard (repeat paste); cut clears
  // it. Returns a summary so the caller can toast.
  async function paste(): Promise<PasteResult> {
    const cb = clipboard.value
    if (!cb)
      return { mode: 'copy', ok: 0, failed: 0 }
    const api = useApi()
    const dir = currentPath.value.replace(/\/$/, '')
    const existing = new Set(files.value.map(f => f.name))
    const sameDirCopy = cb.mode === 'copy' && cb.sourcePath === dir
    const conflicts = cb.names.filter(n => existing.has(n) || sameDirCopy)

    let choice: PasteChoice = 'overwrite'
    if (conflicts.length > 0) {
      choice = await useModalStore().pasteConflict(conflicts)
      if (choice === 'cancel')
        return { mode: cb.mode, ok: 0, failed: 0, cancelled: true }
    }

    // `taken` grows as we append copies so generated names don't collide with
    // each other within the same paste.
    const taken = new Set(existing)
    let ok = 0
    let failed = 0
    for (const name of cb.names) {
      const from = `${cb.sourcePath}/${name}`
      const conflict = existing.has(name) || sameDirCopy
      let toName = name
      if (conflict && choice === 'append')
        toName = uniqueName(name, taken)
      let to = `${dir}/${toName}`
      try {
        if (cb.mode === 'copy') {
          // Never stream a file onto itself (truncates the source) — force a name.
          if (to === from) {
            toName = uniqueName(name, taken)
            to = `${dir}/${toName}`
          }
          await copy(from, to)
        }
        else {
          if (from === to)
            continue // moving onto itself — nothing to do
          if (conflict && choice === 'overwrite')
            await api.del('/api/files', { paths: [to] })
          await move(from, to)
        }
        taken.add(toName)
        ok++
      }
      catch {
        failed++
      }
    }
    await list()
    if (cb.mode === 'cut')
      clearClipboard()
    return { mode: cb.mode, ok, failed }
  }

  async function deleteFiles(paths: string[]): Promise<void> {
    const api = useApi()
    await api.del('/api/files', { paths })
    await list()
  }

  async function mkdir(path: string): Promise<void> {
    const api = useApi()
    await api.post('/api/files/directory', { path })
    await list()
  }

  // Create a directory WITHOUT refreshing the listing — used during folder
  // uploads to materialize empty subdirectories (the upload's own debounced
  // refresh reveals them). Idempotent on the backend (mkdir -p).
  async function ensureDir(path: string): Promise<void> {
    const api = useApi()
    await api.post('/api/files/directory', { path })
  }

  async function chmod(path: string, mode: number): Promise<void> {
    const api = useApi()
    await api.patch('/api/files/permissions', { path, mode })
    await list()
  }

  async function createFile(path: string): Promise<void> {
    const api = useApi()
    const form = new FormData()
    form.append('path', path)
    form.append('file', new Blob([]), 'empty')
    await api.post('/api/files/upload', form)
    await list()
  }

  async function downloadZip(paths: string[]): Promise<void> {
    const authStore = useAuthStore()
    const csrf = authStore.csrfToken
    const resp = await $fetch.raw('/api/files/download-zip', {
      method: 'POST',
      headers: { 'X-CSRF-Token': csrf },
      body: { paths },
      responseType: 'blob',
    })
    const blob = resp._data as Blob
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'archive.zip'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  function $reset() {
    currentPath.value = '/'
    files.value = []
    loading.value = false
    error.value = null
    selected.value = new Set()
    editingName.value = null
    clipboard.value = null
    history.value = []
    historyIndex.value = -1
  }

  const pathSegments = computed(() => {
    const parts = currentPath.value.split('/').filter(Boolean)
    return parts.reduce(
      (acc, part, i) => {
        acc.push({ label: part, path: `/${parts.slice(0, i + 1).join('/')}` })
        return acc
      },
      [] as Array<{ label: string, path: string }>,
    )
  })

  return {
    currentPath,
    files,
    loading,
    error,
    selected,
    editingName,
    clipboard,
    pathSegments,
    canGoBack,
    canGoForward,
    list,
    navigate,
    navigateUp,
    goBack,
    goForward,
    downloadFile,
    downloadUrl,
    fetchObjectUrl,
    toggleSelection,
    clearSelection,
    setSelection,
    startRename,
    cancelRename,
    rename,
    copyToClipboard,
    cutToClipboard,
    clearClipboard,
    copy,
    move,
    paste,
    deleteFiles,
    mkdir,
    ensureDir,
    chmod,
    createFile,
    downloadZip,
    $reset,
  }
})
