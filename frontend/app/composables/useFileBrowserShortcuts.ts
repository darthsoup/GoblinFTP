// Registered from FileTable, so unmounting removes them on /edit and /login.
// defineShortcuts keeps non-`usingInput` bindings inert while a field is focused.

// `visibleNames` is the filtered/sorted set the table shows, so select-all
// matches what the user sees.
export function useFileBrowserShortcuts(visibleNames: () => string[]) {
  const filesStore = useFilesStore()
  const modalStore = useModalStore()
  const paste = usePaste()

  function deleteSelected() {
    if (filesStore.selected.size === 0)
      return
    const dir = filesStore.currentPath.replace(/\/$/, '')
    modalStore.open('delete', { files: [...filesStore.selected].map(name => `${dir}/${name}`) })
  }

  defineShortcuts({
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
    'meta_a': () => filesStore.setSelection(visibleNames()),
    'meta_c': () => filesStore.copyToClipboard([...filesStore.selected]),
    'meta_x': () => filesStore.cutToClipboard([...filesStore.selected]),
    'meta_v': () => {
      if (filesStore.clipboard)
        paste()
    },
    'alt_arrowup': () => filesStore.navigateUp(),
    'alt_arrowleft': () => {
      if (filesStore.canGoBack)
        filesStore.goBack()
    },
    'alt_arrowright': () => {
      if (filesStore.canGoForward)
        filesStore.goForward()
    },
    '?': () => modalStore.open('shortcuts'),
  })
}
