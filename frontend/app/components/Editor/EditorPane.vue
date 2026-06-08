<script setup lang="ts">
import type { LanguageSupport } from '@codemirror/language'
import type { Extension } from '@codemirror/state'
import type { EditorTab } from '~/stores/editor'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { css } from '@codemirror/lang-css'
import { go } from '@codemirror/lang-go'
import { html } from '@codemirror/lang-html'
import { javascript } from '@codemirror/lang-javascript'
import { json } from '@codemirror/lang-json'
import { less } from '@codemirror/lang-less'
import { markdown } from '@codemirror/lang-markdown'
import { php } from '@codemirror/lang-php'
import { python } from '@codemirror/lang-python'
import { sass } from '@codemirror/lang-sass'
import { sql } from '@codemirror/lang-sql'
import { vue } from '@codemirror/lang-vue'
import { xml } from '@codemirror/lang-xml'
import { yaml } from '@codemirror/lang-yaml'
import { defaultHighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { gotoLine, search, searchKeymap } from '@codemirror/search'
import { EditorState } from '@codemirror/state'
import { oneDark } from '@codemirror/theme-one-dark'
import { EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from '@codemirror/view'
import { editorSession } from '~/utils/editorSession'

const editorStore = useEditorStore()
const authStore = useAuthStore()
const colorMode = useColorMode()
const { t } = useI18n()

const containerRef = ref<HTMLElement | null>(null)
const viewOnly = computed(() => authStore.systemVars?.editor?.viewOnly ?? false)
const showEditor = computed(() => !!editorStore.activeTab && !editorStore.activeTab.loading && !editorStore.activeTab.error)

// The view is component-local (recreated per mount); per-tab EditorState +
// scroll live in editorSession so they survive leaving and returning to /edit.
let view: EditorView | null = null
let mountedTabId: string | null = null

function getLanguageExtension(filename: string): LanguageSupport | readonly Extension[] {
  const ext = filename.split('.').pop()?.toLowerCase() ?? ''
  const map: Record<string, LanguageSupport | readonly Extension[]> = {
    js: javascript(),
    mjs: javascript(),
    cjs: javascript(),
    ts: javascript({ typescript: true }),
    jsx: javascript({ jsx: true }),
    tsx: javascript({ jsx: true, typescript: true }),
    html: html(),
    htm: html(),
    xhtml: html(),
    css: css(),
    scss: sass(),
    sass: sass({ indented: true }),
    less: less(),
    vue: vue(),
    php: php(),
    phtml: php(),
    json: json(),
    json5: json(),
    py: python(),
    go: go(),
    sql: sql(),
    xml: xml(),
    svg: xml(),
    yaml: yaml(),
    yml: yaml(),
    md: markdown(),
    markdown: markdown(),
  }
  return map[ext] ?? []
}

// Align CodeMirror's chrome with the Goblin Tech-Dark surfaces (layered over oneDark)
const goblinDarkTheme = EditorView.theme({
  '&': { backgroundColor: '#0d1117' },
  '.cm-scroller': { fontFamily: `'JetBrains Mono Variable', ui-monospace, monospace` },
  '.cm-gutters': { backgroundColor: '#10141a', borderRight: '1px solid #21262d' },
  '.cm-activeLine': { backgroundColor: 'rgba(33, 38, 45, 0.5)' },
  '.cm-activeLineGutter': { backgroundColor: 'rgba(33, 38, 45, 0.5)' },
  '.cm-panels': { backgroundColor: '#10141a', color: '#c9d1d9' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid #21262d' },
  '.cm-textfield': { backgroundColor: '#0d1117', border: '1px solid #21262d', color: '#c9d1d9' },
  '.cm-button': { backgroundColor: '#21262d', border: '1px solid #30363d', color: '#c9d1d9', backgroundImage: 'none' },
}, { dark: true })

// Goblin Tech-Light: recessed code surface (#f1f5f9) with slate chrome
const goblinLightTheme = EditorView.theme({
  '&': { backgroundColor: '#f1f5f9' },
  '.cm-scroller': { fontFamily: `'JetBrains Mono Variable', ui-monospace, monospace` },
  '.cm-gutters': { backgroundColor: '#e6e8ea', borderRight: '1px solid #cbd5e1', color: '#64748b' },
  '.cm-activeLine': { backgroundColor: 'rgba(203, 213, 225, 0.35)' },
  '.cm-activeLineGutter': { backgroundColor: 'rgba(203, 213, 225, 0.35)' },
  '.cm-panels': { backgroundColor: '#e6e8ea', color: '#334155' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid #cbd5e1' },
  '.cm-textfield': { backgroundColor: '#ffffff', border: '1px solid #cbd5e1', color: '#334155' },
  '.cm-button': { backgroundColor: '#f1f5f9', border: '1px solid #cbd5e1', color: '#334155', backgroundImage: 'none' },
}, { dark: false })

function themeExtensions(): Extension {
  if (colorMode.value === 'light')
    return [syntaxHighlighting(defaultHighlightStyle), goblinLightTheme]
  return [syntaxHighlighting(defaultHighlightStyle), oneDark, goblinDarkTheme]
}

function saveActive() {
  if (viewOnly.value || !editorStore.activeId)
    return
  editorStore.saveTab(editorStore.activeId)
}

function buildExtensions(filename: string): Extension[] {
  return [
    lineNumbers(),
    highlightActiveLine(),
    highlightActiveLineGutter(),
    history(),
    search({ top: true }),
    editorSession.themeCompartment.of(themeExtensions()),
    keymap.of([
      ...searchKeymap,
      ...defaultKeymap,
      ...historyKeymap,
      indentWithTab,
      { key: 'Mod-Alt-g', run: gotoLine },
      {
        key: 'Mod-s',
        run() {
          saveActive()
          return true
        },
      },
    ]),
    getLanguageExtension(filename),
    EditorState.readOnly.of(viewOnly.value),
    EditorView.updateListener.of((update) => {
      // Route real edits to the tab whose state is mounted; ignore programmatic
      // setState swaps (those produce an update with no transactions).
      if (update.docChanged && update.transactions.length > 0 && mountedTabId)
        editorStore.updateContent(mountedTabId, update.state.doc.toString())
    }),
  ]
}

function createState(tab: EditorTab): EditorState {
  return EditorState.create({ doc: tab.content, extensions: buildExtensions(tab.name) })
}

function snapshotMounted() {
  if (view && mountedTabId && editorStore.tabs.some(tab => tab.id === mountedTabId)) {
    editorSession.tabStates.set(mountedTabId, view.state)
    editorSession.tabScroll.set(mountedTabId, view.scrollDOM.scrollTop)
  }
}

function sync() {
  if (!containerRef.value)
    return

  // Drop cached state for tabs that have been closed.
  for (const id of editorSession.tabStates.keys()) {
    if (!editorStore.tabs.some(tab => tab.id === id)) {
      editorSession.tabStates.delete(id)
      editorSession.tabScroll.delete(id)
    }
  }

  const tab = editorStore.activeTab
  if (mountedTabId && (!tab || tab.id !== mountedTabId)) {
    snapshotMounted()
    mountedTabId = null
  }

  // Nothing editable to show (no tab, still loading, or errored): keep the view
  // alive (it holds the last state) but it's hidden via showEditor.
  if (!tab || tab.loading || tab.error)
    return

  if (mountedTabId === tab.id)
    return

  const state = editorSession.tabStates.get(tab.id) ?? createState(tab)
  if (!view)
    view = new EditorView({ state, parent: containerRef.value })
  else
    view.setState(state)
  // Sync the theme to the current mode (a restored state may carry a stale one).
  view.dispatch({ effects: editorSession.themeCompartment.reconfigure(themeExtensions()) })
  mountedTabId = tab.id

  const top = editorSession.tabScroll.get(tab.id) ?? 0
  requestAnimationFrame(() => {
    if (view)
      view.scrollDOM.scrollTop = top
  })
}

watch(
  () => [editorStore.activeId, editorStore.activeTab?.loading, editorStore.activeTab?.error] as const,
  async () => {
    await nextTick()
    sync()
  },
  { immediate: true },
)

watch(() => colorMode.value, () => {
  view?.dispatch({ effects: editorSession.themeCompartment.reconfigure(themeExtensions()) })
})

onUnmounted(() => {
  // Persist the mounted tab's state before tearing down the view, so a route
  // round-trip back to /edit restores undo/cursor/scroll. The maps live in
  // editorSession (cleared only on editor $reset).
  snapshotMounted()
  view?.destroy()
  view = null
  mountedTabId = null
})
</script>

<template>
  <div class="flex flex-col flex-1 min-h-0 overflow-hidden">
    <EditorTabBar />

    <div class="relative flex-1 min-h-0">
      <div v-show="showEditor" ref="containerRef" class="absolute inset-0 overflow-auto" />

      <div v-if="editorStore.activeTab?.loading" class="absolute inset-0 flex items-center justify-center text-muted font-mono text-sm">
        <UIcon name="i-lucide-loader-circle" class="size-5 animate-spin mr-2 text-primary" />
        {{ t('editor.loading') }}
      </div>

      <div v-else-if="editorStore.activeTab?.error" class="absolute inset-0 flex items-center justify-center text-error font-mono text-sm">
        <UIcon name="i-lucide-circle-x" class="size-5 mr-2" />
        {{ editorStore.activeTab.error }}
      </div>

      <div v-else-if="!editorStore.hasOpenTabs" class="absolute inset-0 flex items-center justify-center text-dimmed font-mono text-sm">
        {{ t('editor.noFile') }}
      </div>
    </div>
  </div>
</template>

<style>
.cm-editor {
  height: 100%;
}

/* Doubled class beats the theme-generated selectors (oneDark ties with the
   goblin chrome themes otherwise) */
.cm-editor.cm-editor {
  background-color: var(--gftp-editor-bg);
}

.cm-scroller {
  overflow: auto;
}
</style>
