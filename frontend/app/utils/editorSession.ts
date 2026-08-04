import type { EditorState } from '@codemirror/state'
import { Compartment } from '@codemirror/state'

// Per-tab CodeMirror state that must outlive EditorPane remounts.
export const editorSession = {
  tabStates: new Map<string, EditorState>(),
  tabScroll: new Map<string, number>(),
  themeCompartment: new Compartment(),
  languageCompartment: new Compartment(),
}

// Discard a tab's cached state so the next sync rebuilds it from tab.content.
// Called after a reload replaces the buffer: the cached EditorState still holds
// the pre-reload document, and restoring it would silently resurrect the stale
// text the user just chose to discard.
export function dropTabState(id: string) {
  editorSession.tabStates.delete(id)
  editorSession.tabScroll.delete(id)
}

export function clearEditorSession() {
  editorSession.tabStates.clear()
  editorSession.tabScroll.clear()
}
