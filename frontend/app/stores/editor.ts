import { defineStore } from 'pinia'
import { clearEditorSession } from '~/utils/editorSession'

export interface EditorTab {
  id: string
  path: string
  name: string
  content: string
  savedContent: string
  loading: boolean
  saving: boolean
  error?: string
}

const AUTOSAVE_DELAY_MS = 2000

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

  async function saveTab(id: string) {
    const tab = tabs.value.find(t => t.id === id)
    if (!tab || tab.saving)
      return
    clearAutoSave(id)
    tab.saving = true
    tab.error = undefined
    try {
      const api = useApi()
      await api.post('/api/files/write', { path: tab.path, content: tab.content })
      tab.savedContent = tab.content
    }
    catch (e) {
      tab.error = e instanceof Error ? e.message : 'Failed to save'
    }
    finally {
      tab.saving = false
    }
  }

  function scheduleAutoSave(id: string) {
    clearAutoSave(id)
    if (!useSettingsStore().editorAutoSave)
      return
    autoSaveTimers.set(id, setTimeout(() => {
      autoSaveTimers.delete(id)
      const tab = tabs.value.find(t => t.id === id)
      if (useSettingsStore().editorAutoSave && tab && !tab.loading && !tab.saving && tab.content !== tab.savedContent)
        saveTab(id)
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
    const tab: EditorTab = { id, path, name, content: '', savedContent: '', loading: true, saving: false }
    tabs.value = [...tabs.value, tab]
    activeId.value = id

    try {
      const api = useApi()
      const data = await api.get<{ content: string, path: string }>(`/api/files/read?path=${encodeURIComponent(path)}`)
      const t = tabs.value.find(t => t.id === id)
      if (t) {
        t.content = data.content
        t.savedContent = data.content
        t.loading = false
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

  return { tabs, activeId, activeTab, hasOpenTabs, dirtyCount, hasDirty, openFile, saveTab, updateContent, closeTab, setActive, $reset }
})
