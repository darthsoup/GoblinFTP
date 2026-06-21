import type { EditorState } from '@codemirror/state'
import { Compartment } from '@codemirror/state'

// Per-tab CodeMirror state that must outlive EditorPane remounts.
export const editorSession = {
  tabStates: new Map<string, EditorState>(),
  tabScroll: new Map<string, number>(),
  themeCompartment: new Compartment(),
  languageCompartment: new Compartment(),
}

export function clearEditorSession() {
  editorSession.tabStates.clear()
  editorSession.tabScroll.clear()
}
