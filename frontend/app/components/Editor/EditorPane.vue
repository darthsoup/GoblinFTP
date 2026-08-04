<script setup lang="ts">
import type { Extension } from '@codemirror/state'
import type { EditorTab } from '~/stores/editor'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
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
const conflictTab = computed(() => (editorStore.activeTab?.conflict ? editorStore.activeTab : null))

// The view is component-local (recreated per mount); per-tab EditorState +
// scroll live in editorSession so they survive leaving and returning to /edit.
let view: EditorView | null = null
let mountedTabId: string | null = null
// The tab.revision the mounted state was built from. A reload bumps the tab's
// revision, and without comparing it sync() would early-return on the unchanged
// tab id and keep showing (and then saving) the document the user discarded.
let mountedRevision = -1

// Each grammar is dynamic-imported so only the one a file needs is fetched (and
// the 14 grammars stay out of the editor's main chunk). Applied after mount via
// editorSession.languageCompartment.
async function loadLanguage(filename: string): Promise<Extension> {
  const ext = filename.split('.').pop()?.toLowerCase() ?? ''
  switch (ext) {
    case 'js':
    case 'mjs':
    case 'cjs':
      return (await import('@codemirror/lang-javascript')).javascript()
    case 'ts':
      return (await import('@codemirror/lang-javascript')).javascript({ typescript: true })
    case 'jsx':
      return (await import('@codemirror/lang-javascript')).javascript({ jsx: true })
    case 'tsx':
      return (await import('@codemirror/lang-javascript')).javascript({ jsx: true, typescript: true })
    case 'html':
    case 'htm':
    case 'xhtml':
      return (await import('@codemirror/lang-html')).html()
    case 'css':
      return (await import('@codemirror/lang-css')).css()
    case 'scss':
      return (await import('@codemirror/lang-sass')).sass()
    case 'sass':
      return (await import('@codemirror/lang-sass')).sass({ indented: true })
    case 'less':
      return (await import('@codemirror/lang-less')).less()
    case 'vue':
      return (await import('@codemirror/lang-vue')).vue()
    case 'php':
    case 'phtml':
      return (await import('@codemirror/lang-php')).php()
    case 'json':
    case 'json5':
      return (await import('@codemirror/lang-json')).json()
    case 'py':
      return (await import('@codemirror/lang-python')).python()
    case 'go':
      return (await import('@codemirror/lang-go')).go()
    case 'sql':
      return (await import('@codemirror/lang-sql')).sql()
    case 'xml':
    case 'svg':
      return (await import('@codemirror/lang-xml')).xml()
    case 'yaml':
    case 'yml':
      return (await import('@codemirror/lang-yaml')).yaml()
    case 'md':
    case 'markdown':
      return (await import('@codemirror/lang-markdown')).markdown()
    default:
      return []
  }
}

// Align CodeMirror's chrome with the Goblin design tokens (layered over oneDark in
// dark mode). The themes are mode-scoped, so the design-system custom properties —
// which flip per mode — resolve to the right surface here.
const goblinDarkTheme = EditorView.theme({
  '&': { backgroundColor: 'var(--ui-bg)' },
  '.cm-scroller': { fontFamily: `'JetBrains Mono Variable', ui-monospace, monospace` },
  '.cm-gutters': { backgroundColor: 'var(--ui-bg-muted)', borderRight: '1px solid var(--ui-border)' },
  '.cm-activeLine': { backgroundColor: 'color-mix(in oklab, var(--ui-bg-accented) 50%, transparent)' },
  '.cm-activeLineGutter': { backgroundColor: 'color-mix(in oklab, var(--ui-bg-accented) 50%, transparent)' },
  '.cm-panels': { backgroundColor: 'var(--ui-bg-muted)', color: 'var(--ui-text)' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--ui-border)' },
  '.cm-textfield': { backgroundColor: 'var(--ui-bg)', border: '1px solid var(--ui-border)', color: 'var(--ui-text)' },
  '.cm-button': { backgroundColor: 'var(--ui-bg-accented)', border: '1px solid var(--ui-border-accented)', color: 'var(--ui-text)', backgroundImage: 'none' },
}, { dark: true })

// Goblin Tech-Light: recessed code surface with slate chrome (same tokens, light values).
const goblinLightTheme = EditorView.theme({
  '&': { backgroundColor: 'var(--gftp-editor-bg)' },
  '.cm-scroller': { fontFamily: `'JetBrains Mono Variable', ui-monospace, monospace` },
  '.cm-gutters': { backgroundColor: 'var(--ui-bg-accented)', borderRight: '1px solid var(--ui-border-accented)', color: 'var(--ui-text-muted)' },
  '.cm-activeLine': { backgroundColor: 'color-mix(in oklab, var(--ui-border-accented) 35%, transparent)' },
  '.cm-activeLineGutter': { backgroundColor: 'color-mix(in oklab, var(--ui-border-accented) 35%, transparent)' },
  '.cm-panels': { backgroundColor: 'var(--ui-bg-accented)', color: 'var(--ui-text)' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--ui-border-accented)' },
  '.cm-textfield': { backgroundColor: 'var(--ui-bg-elevated)', border: '1px solid var(--ui-border-accented)', color: 'var(--ui-text)' },
  '.cm-button': { backgroundColor: 'var(--gftp-editor-bg)', border: '1px solid var(--ui-border-accented)', color: 'var(--ui-text)', backgroundImage: 'none' },
}, { dark: false })

function themeExtensions(): Extension {
  if (colorMode.value === 'light')
    return [syntaxHighlighting(defaultHighlightStyle), goblinLightTheme]
  return [syntaxHighlighting(defaultHighlightStyle), oneDark, goblinDarkTheme]
}

// interactive: an explicit save is a request the user is waiting on, so a
// refusal opens the dialog. Autosave only sets the tab's conflict state and
// lets the banner offer the same choices.
function saveActive() {
  if (viewOnly.value || !editorStore.activeId)
    return
  editorStore.saveTab(editorStore.activeId, { interactive: true })
}

function buildExtensions(): Extension[] {
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
    editorSession.languageCompartment.of([]),
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
  return EditorState.create({ doc: tab.content, extensions: buildExtensions() })
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
    mountedRevision = -1
  }

  // Nothing editable to show (no tab, still loading, or errored): keep the view
  // alive (it holds the last state) but it's hidden via showEditor.
  if (!tab || tab.loading || tab.error)
    return

  if (mountedTabId === tab.id && mountedRevision === tab.revision)
    return

  const restored = editorSession.tabStates.get(tab.id)
  const state = restored ?? createState(tab)
  if (!view)
    view = new EditorView({ state, parent: containerRef.value })
  else
    view.setState(state)
  // Sync the theme to the current mode (a restored state may carry a stale one).
  view.dispatch({ effects: editorSession.themeCompartment.reconfigure(themeExtensions()) })
  mountedTabId = tab.id
  mountedRevision = tab.revision

  // Fresh states start without a grammar — load it lazily and swap it in, guarded
  // so a slow import for a since-switched tab can't apply to the wrong document.
  // Restored states already carry their grammar in the compartment.
  if (!restored) {
    void loadLanguage(tab.name).then((lang) => {
      if (view && mountedTabId === tab.id)
        view.dispatch({ effects: editorSession.languageCompartment.reconfigure(lang) })
    })
  }

  const top = editorSession.tabScroll.get(tab.id) ?? 0
  requestAnimationFrame(() => {
    if (view)
      view.scrollDOM.scrollTop = top
  })
}

watch(
  () => [
    editorStore.activeId,
    editorStore.activeTab?.loading,
    editorStore.activeTab?.error,
    editorStore.activeTab?.revision,
  ] as const,
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

    <!-- Conflict banner. Deliberately outside the editor region so the user's
         unsaved buffer stays visible, and it persists after the dialog is
         dismissed so autosave is never silently off with no explanation. -->
    <div
      v-if="conflictTab"
      class="shrink-0 flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2 border-b border-default bg-warning/10"
    >
      <UIcon name="i-lucide-triangle-alert" class="size-4 shrink-0 text-warning" />
      <span class="text-sm text-default">
        {{ conflictTab.conflict === 'deleted' ? t('editor.conflict.bannerDeleted') : t('editor.conflict.banner') }}
      </span>
      <div class="ml-auto flex items-center gap-1.5">
        <UButton
          v-if="conflictTab.conflict !== 'deleted'"
          size="xs"
          color="neutral"
          variant="outline"
          :label="t('editor.conflict.reload')"
          @click="editorStore.reloadTab(conflictTab.id)"
        />
        <UButton
          size="xs"
          color="error"
          variant="soft"
          :label="t('editor.conflict.overwrite')"
          @click="editorStore.saveTab(conflictTab.id, { force: true })"
        />
      </div>
    </div>

    <div class="relative flex-1 min-h-0">
      <div v-show="showEditor" ref="containerRef" class="absolute inset-0 overflow-auto" />

      <div v-if="editorStore.activeTab?.loading" class="absolute inset-0 flex items-center justify-center text-muted text-sm">
        <UIcon name="i-lucide-loader-circle" class="size-5 animate-spin mr-2 text-primary" />
        {{ t('editor.loading') }}
      </div>

      <div v-else-if="editorStore.activeTab?.error" class="absolute inset-0 flex items-center justify-center text-error text-sm">
        <UIcon name="i-lucide-circle-x" class="size-5 mr-2" />
        {{ editorStore.activeTab.error }}
      </div>

      <div v-else-if="!editorStore.hasOpenTabs" class="absolute inset-0 flex items-center justify-center text-dimmed text-sm">
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
