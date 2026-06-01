import { defineStore } from 'pinia'

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

export const useEditorStore = defineStore('editor', () => {
  const tabs = ref<EditorTab[]>([])
  const activeId = ref<string | null>(null)

  const activeTab = computed(() => tabs.value.find(t => t.id === activeId.value) ?? null)
  const hasOpenTabs = computed(() => tabs.value.length > 0)

  async function openFile(path: string) {
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

  async function saveTab(id: string) {
    const tab = tabs.value.find(t => t.id === id)
    if (!tab || tab.saving)
      return
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

  function updateContent(id: string, content: string) {
    const tab = tabs.value.find(t => t.id === id)
    if (tab)
      tab.content = content
  }

  function closeTab(id: string) {
    const idx = tabs.value.findIndex(t => t.id === id)
    if (idx === -1)
      return
    tabs.value = tabs.value.filter(t => t.id !== id)
    if (activeId.value === id) {
      activeId.value = tabs.value[Math.min(idx, tabs.value.length - 1)]?.id ?? null
    }
  }

  function setActive(id: string) {
    activeId.value = id
  }

  function $reset() {
    tabs.value = []
    activeId.value = null
  }

  return { tabs, activeId, activeTab, hasOpenTabs, openFile, saveTab, updateContent, closeTab, setActive, $reset }
})
