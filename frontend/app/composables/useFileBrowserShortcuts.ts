// File-browser keyboard shortcuts. Registered from FileTable (browser page only;
// auto-removed on unmount → inactive on /edit and /login). Shortcuts without
// `usingInput` are disabled by defineShortcuts while a field is focused, so they
// never fire mid-rename or while typing in a modal. `visibleNames` is the
// filtered/sorted set the table shows, so select-all matches what the user sees.
export function useFileBrowserShortcuts(visibleNames: () => string[]) {
  const filesStore = useFilesStore()
  const modalStore = useModalStore()

  function deleteSelected() {
    if (filesStore.selected.size === 0)
      return
    const dir = filesStore.currentPath.replace(/\/$/, '')
    modalStore.open('delete', { files: [...filesStore.selected].map(name => `${dir}/${name}`) })
  }

  defineShortcuts({
    // Action keys
    'f2': () => {
      if (filesStore.selected.size === 1)
        filesStore.startRename([...filesStore.selected][0]!)
    },
    'delete': deleteSelected,
    'meta_backspace': deleteSelected, // macOS "move to trash" (→ Ctrl+Backspace off-Mac)
    'escape': () => {
      if (filesStore.editingName)
        filesStore.cancelRename()
      else
        filesStore.clearSelection()
    },
    // Select all (Cmd/Ctrl+A — auto-mapped to Ctrl off-Mac)
    'meta_a': () => filesStore.setSelection(visibleNames()),
    // Navigation
    'alt_arrowup': () => filesStore.navigateUp(),
    'alt_arrowleft': () => {
      if (filesStore.canGoBack)
        filesStore.goBack()
    },
    'alt_arrowright': () => {
      if (filesStore.canGoForward)
        filesStore.goForward()
    },
    // Help overlay
    '?': () => modalStore.open('shortcuts'),
  })
}
