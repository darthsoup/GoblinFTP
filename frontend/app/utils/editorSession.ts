import type { EditorState } from '@codemirror/state'
import { Compartment } from '@codemirror/state'

// Per-tab CodeMirror state that must outlive EditorPane remounts (route round-
// trips through /edit) so undo history, cursor/selection, and scroll survive
// leaving and returning to the editor. The view itself is component-local
// (recreated per mount from the active tab's saved state here); these maps are
// the source of truth. Cleared on editor $reset (disconnect / session loss).
export const editorSession = {
  tabStates: new Map<string, EditorState>(),
  tabScroll: new Map<string, number>(),
  // Stable across remounts so a theme reconfigure targets restored states.
  themeCompartment: new Compartment(),
}

export function clearEditorSession() {
  editorSession.tabStates.clear()
  editorSession.tabScroll.clear()
}
