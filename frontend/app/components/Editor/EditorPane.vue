<script setup lang="ts">
import type { LanguageSupport } from '@codemirror/language'
import type { Extension } from '@codemirror/state'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { css } from '@codemirror/lang-css'
import { html } from '@codemirror/lang-html'
import { javascript } from '@codemirror/lang-javascript'
import { json } from '@codemirror/lang-json'
import { markdown } from '@codemirror/lang-markdown'
import { python } from '@codemirror/lang-python'
import { xml } from '@codemirror/lang-xml'
import { defaultHighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { EditorState } from '@codemirror/state'
import { oneDark } from '@codemirror/theme-one-dark'
import { EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from '@codemirror/view'

const editorStore = useEditorStore()
const authStore = useAuthStore()
const { t } = useI18n()

const containerRef = ref<HTMLElement | null>(null)
const autoSave = ref(false)
let view: EditorView | null = null
let autoSaveTimer: ReturnType<typeof setTimeout> | null = null

const viewOnly = computed(() => authStore.systemVars?.editor?.viewOnly ?? false)

function clearAutoSaveTimer() {
  if (autoSaveTimer) {
    clearTimeout(autoSaveTimer)
    autoSaveTimer = null
  }
}

function getLanguageExtension(filename: string): LanguageSupport | readonly Extension[] {
  const ext = filename.split('.').pop()?.toLowerCase() ?? ''
  const map: Record<string, LanguageSupport | readonly Extension[]> = {
    js: javascript(),
    ts: javascript({ typescript: true }),
    jsx: javascript({ jsx: true }),
    tsx: javascript({ jsx: true, typescript: true }),
    html: html(),
    htm: html(),
    css: css(),
    json: json(),
    py: python(),
    xml: xml(),
    md: markdown(),
    markdown: markdown(),
  }
  return map[ext] ?? []
}

function buildExtensions(filename: string, readOnly: boolean): Extension[] {
  return [
    lineNumbers(),
    highlightActiveLine(),
    highlightActiveLineGutter(),
    history(),
    syntaxHighlighting(defaultHighlightStyle),
    oneDark,
    keymap.of([
      ...defaultKeymap,
      ...historyKeymap,
      indentWithTab,
      {
        key: 'Mod-s',
        run() {
          if (editorStore.activeId)
            editorStore.saveTab(editorStore.activeId)
          return true
        },
      },
    ]),
    getLanguageExtension(filename),
    EditorState.readOnly.of(readOnly),
    EditorView.updateListener.of((update) => {
      if (update.docChanged && editorStore.activeId) {
        const tabId = editorStore.activeId
        editorStore.updateContent(tabId, update.state.doc.toString())
        if (autoSave.value) {
          clearAutoSaveTimer()
          autoSaveTimer = setTimeout(() => {
            const tab = editorStore.tabs.find(candidate => candidate.id === tabId)
            if (tab && !tab.loading && tab.content !== tab.savedContent)
              editorStore.saveTab(tabId)
          }, 2000)
        }
      }
    }),
  ]
}

function destroyEditor() {
  view?.destroy()
  view = null
  clearAutoSaveTimer()
}

function mountEditor(content: string, filename: string) {
  destroyEditor()
  if (!containerRef.value)
    return
  view = new EditorView({
    state: EditorState.create({
      doc: content,
      extensions: buildExtensions(filename, viewOnly.value),
    }),
    parent: containerRef.value,
  })
}

watch(
  () => [editorStore.activeId, editorStore.activeTab?.loading, editorStore.activeTab?.error, viewOnly.value] as const,
  async ([activeId, loading, error]) => {
    if (!activeId || loading || error) {
      destroyEditor()
      return
    }

    await nextTick()
    const tab = editorStore.activeTab
    if (!tab || tab.id !== activeId || tab.loading || tab.error)
      return

    mountEditor(tab.content, tab.name)
  },
  { immediate: true },
)

watch(autoSave, (enabled) => {
  if (!enabled)
    clearAutoSaveTimer()
})

onUnmounted(() => {
  destroyEditor()
})
</script>

<template>
  <div class="flex flex-col flex-1 overflow-hidden">
    <EditorTabBar :auto-save="autoSave" @toggle-auto-save="autoSave = !autoSave" />

    <div v-if="editorStore.activeTab?.loading" class="flex items-center justify-center flex-1 text-gray-400">
      <UIcon name="i-heroicons-arrow-path" class="w-6 h-6 animate-spin mr-2" />
      {{ t('editor.loading') }}
    </div>

    <div v-else-if="editorStore.activeTab?.error" class="flex items-center justify-center flex-1 text-red-500">
      <UIcon name="i-heroicons-exclamation-circle" class="w-5 h-5 mr-2" />
      {{ editorStore.activeTab.error }}
    </div>

    <div v-else-if="!editorStore.hasOpenTabs" class="flex items-center justify-center flex-1 text-gray-400">
      {{ t('editor.noFile') }}
    </div>

    <div v-else ref="containerRef" class="flex-1 overflow-auto" />
  </div>
</template>

<style>
.cm-editor {
  height: 100%;
}

.cm-scroller {
  overflow: auto;
}
</style>
