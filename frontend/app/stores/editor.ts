import type { EditorConflictKind } from '~/stores/modal'
import type { FileVersion, ReadFileResult } from '~/types/api'
import { defineStore } from 'pinia'
import { ApiError } from '~/types/api'
import { clearEditorSession, dropTabState } from '~/utils/editorSession'

export interface EditorTab {
  id: string
  path: string
  name: string
  content: string
  savedContent: string
  loading: boolean
  saving: boolean
  error?: string
  // Opaque server token for the copy this tab was opened from. undefined means
  // the file was never read (nothing to precondition on); null means the server
  // could not stat it, so saves go through unconditionally as they used to.
  version?: string | null
  baselineSize?: number
  baselineModified?: string
  // Set when the server refused a save. Pauses autosave and shows the banner.
  conflict?: EditorConflictKind
  // Bumped by reloadTab so EditorPane rebuilds its CodeMirror state instead of
  // restoring the cached one, which would keep showing the stale document.
  revision: number
  // A conflict is re-prompted once; a second one in a row is an error rather
  // than another dialog, so a racing server cannot loop it. Mirrors upload.ts.
  reprompted?: boolean
}

const AUTOSAVE_DELAY_MS = 2000
// Open tabs are persisted (paths only) so a reload reopens them, re-fetching
// content fresh. Cleared when the editor is reset (disconnect / session loss).
const STORAGE_KEY = 'gftp_editor_tabs'

export const useEditorStore = defineStore('editor', () => {
  const tabs = ref<EditorTab[]>([])
  const activeId = ref<string | null>(null)

  const activeTab = computed(() => tabs.value.find(t => t.id === activeId.value) ?? null)
  const hasOpenTabs = computed(() => tabs.value.length > 0)
  const dirtyCount = computed(() => tabs.value.filter(t => t.content !== t.savedContent).length)
  const hasDirty = computed(() => dirtyCount.value > 0)

  // Per-tab autosave debounce timers — non-reactive on purpose (one timer per
  // tab so switching tabs never cancels another tab's pending save).
  const autoSaveTimers = new Map<string, ReturnType<typeof setTimeout>>()

  // Suppresses persistence while restore() is repopulating tabs.
  let restoring = false

  function clearAutoSave(id: string) {
    const timer = autoSaveTimers.get(id)
    if (timer) {
      clearTimeout(timer)
      autoSaveTimers.delete(id)
    }
  }

  function clearAllAutoSave() {
    for (const timer of autoSaveTimers.values())
      clearTimeout(timer)
    autoSaveTimers.clear()
  }

  function adoptVersion(tab: EditorTab, meta: FileVersion | undefined) {
    if (!meta)
      return
    tab.version = meta.version
    tab.baselineSize = meta.size
    tab.baselineModified = meta.modified
  }

  // force skips the server-side precondition; the user was shown the conflict
  // and chose to replace. interactive decides whether a refusal raises the modal
  // or only the inline banner — autosave must never interrupt mid-keystroke.
  async function saveTab(id: string, { interactive = false, force = false } = {}) {
    const tab = tabs.value.find(t => t.id === id)
    if (!tab || tab.saving || tab.loading)
      return
    // No baseline means the read never completed. Saving here would write the
    // empty buffer over a healthy file; the backend rejects it anyway.
    if (tab.version === undefined)
      return
    clearAutoSave(id)
    tab.saving = true
    tab.error = undefined
    const content = tab.content
    try {
      const api = useApi()
      // A null version means the server offers no conflict detection for this
      // file, so there is nothing to precondition on.
      const meta = await api.post<FileVersion>('/api/files/write', force || tab.version === null
        ? { path: tab.path, content, overwrite: true }
        : { path: tab.path, content, expectedVersion: tab.version })
      tab.savedContent = content
      tab.conflict = undefined
      tab.reprompted = false
      adoptVersion(tab, meta)
    }
    catch (e) {
      const code = e instanceof ApiError ? e.code : ''
      const kind: EditorConflictKind | null
        = code === 'ERR_FILE_MODIFIED' ? 'modified' : code === 'ERR_FILE_NOT_FOUND' ? 'deleted' : null
      if (kind && !tab.reprompted) {
        tab.conflict = kind
        tab.saving = false
        if (interactive)
          await resolveConflict(id)
        return
      }
      tab.error = e instanceof Error ? e.message : 'Failed to save'
    }
    finally {
      tab.saving = false
    }
  }

  // Asks what to do about a refused save. Only reached from an explicit save;
  // autosave leaves tab.conflict set and lets the banner offer the same choices.
  async function resolveConflict(id: string) {
    const tab = tabs.value.find(t => t.id === id)
    if (!tab?.conflict)
      return
    const choice = await useModalStore().editorConflict({
      name: tab.name,
      kind: tab.conflict,
      size: tab.baselineSize,
      modified: tab.baselineModified,
    })
    if (choice === 'overwrite') {
      tab.reprompted = true
      await saveTab(id, { force: true })
      tab.reprompted = false
    }
    else if (choice === 'reload') {
      await reloadTab(id)
    }
    // 'cancel' keeps the buffer and leaves tab.conflict set, so the banner
    // explains why autosave stopped.
  }

  // Replaces the buffer with the server's copy and re-pins the baseline. The
  // revision bump is what makes EditorPane rebuild its editor state.
  async function reloadTab(id: string) {
    const tab = tabs.value.find(t => t.id === id)
    if (!tab)
      return
    tab.loading = true
    try {
      const api = useApi()
      const data = await api.get<ReadFileResult>(`/api/files/read?path=${encodeURIComponent(tab.path)}`)
      const t = tabs.value.find(t => t.id === id)
      if (!t)
        return
      t.content = data.content
      t.savedContent = data.content
      t.conflict = undefined
      t.reprompted = false
      t.error = undefined
      // Both matter: dropping the cached CodeMirror state stops the stale
      // document being restored, and the revision bump is what tells EditorPane
      // to rebuild rather than early-return on the unchanged tab id.
      dropTabState(id)
      t.revision++
      adoptVersion(t, data)
    }
    catch (e) {
      const t = tabs.value.find(t => t.id === id)
      if (t)
        t.error = e instanceof Error ? e.message : 'Failed to reload file'
    }
    finally {
      const t = tabs.value.find(t => t.id === id)
      if (t)
        t.loading = false
    }
  }

  function scheduleAutoSave(id: string) {
    clearAutoSave(id)
    if (!useSettingsStore().editorAutoSave)
      return
    autoSaveTimers.set(id, setTimeout(() => {
      autoSaveTimers.delete(id)
      const tab = tabs.value.find(t => t.id === id)
      // An unresolved conflict pauses autosave: retrying on every keystroke
      // pause would hammer the server and the user has already been told.
      if (useSettingsStore().editorAutoSave && tab && !tab.loading && !tab.saving
        && !tab.conflict && tab.content !== tab.savedContent) {
        saveTab(id)
      }
    }, AUTOSAVE_DELAY_MS))
  }

  async function openFile(path: string) {
    // The tab is pushed synchronously before the first await, so a rapid second
    // call finds it here — no duplicate-tab race.
    const existing = tabs.value.find(t => t.path === path)
    if (existing) {
      activeId.value = existing.id
      return
    }

    const id = crypto.randomUUID()
    const name = path.split('/').pop() ?? path
    const tab: EditorTab = { id, path, name, content: '', savedContent: '', loading: true, saving: false, revision: 0 }
    tabs.value = [...tabs.value, tab]
    activeId.value = id

    try {
      const api = useApi()
      const data = await api.get<ReadFileResult>(`/api/files/read?path=${encodeURIComponent(path)}`)
      const t = tabs.value.find(t => t.id === id)
      if (t) {
        t.content = data.content
        t.savedContent = data.content
        t.loading = false
        adoptVersion(t, data)
      }
    }
    catch (e) {
      const t = tabs.value.find(t => t.id === id)
      if (t) {
        t.loading = false
        t.error = e instanceof Error ? e.message : 'Failed to load file'
      }
    }
  }

  function updateContent(id: string, content: string) {
    const tab = tabs.value.find(t => t.id === id)
    if (!tab)
      return
    tab.content = content
    scheduleAutoSave(id)
  }

  function closeTab(id: string) {
    const idx = tabs.value.findIndex(t => t.id === id)
    if (idx === -1)
      return
    clearAutoSave(id)
    tabs.value = tabs.value.filter(t => t.id !== id)
    if (activeId.value === id)
      activeId.value = tabs.value[Math.min(idx, tabs.value.length - 1)]?.id ?? null
  }

  function setActive(id: string) {
    activeId.value = id
  }

  function $reset() {
    clearAllAutoSave()
    clearEditorSession()
    tabs.value = []
    activeId.value = null
  }

  // ── Reload persistence (paths only) ────────────────────────────────────────
  function persist() {
    if (restoring)
      return
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        paths: tabs.value.map(t => t.path),
        activePath: activeTab.value?.path ?? null,
      }))
    }
    catch {}
  }

  // Reopen the previously-open tabs after a reload. Caller must ensure the
  // session is connected (paths are re-fetched). Tabs that fail to reload (e.g.
  // the file was removed) are dropped.
  async function restore() {
    if (tabs.value.length)
      return
    let saved: { paths?: string[], activePath?: string | null } | null = null
    try {
      saved = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null')
    }
    catch {}
    if (!saved?.paths?.length)
      return

    restoring = true
    try {
      for (const path of saved.paths)
        await openFile(path)
      tabs.value = tabs.value.filter(t => !t.error)
      const target = saved.activePath ? tabs.value.find(t => t.path === saved.activePath) : null
      activeId.value = target?.id ?? tabs.value[0]?.id ?? null
    }
    finally {
      restoring = false
      persist()
    }
  }

  // Persist when the set of open tabs or the active tab changes (not on edits).
  watch([() => tabs.value.map(t => t.path).join('\n'), activeId], () => persist())

  return { tabs, activeId, activeTab, hasOpenTabs, dirtyCount, hasDirty, openFile, saveTab, resolveConflict, reloadTab, updateContent, closeTab, setActive, restore, $reset }
})
