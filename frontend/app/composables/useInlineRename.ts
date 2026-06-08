// Shared in-place rename behaviour for FileRow (table) and FileCard (cards):
// seeds a draft from the file name when editing starts, focuses + selects the
// base name, and dedupes the trailing blur that fires when the input unmounts
// after Enter/Esc.
export function useInlineRename(opts: {
  editing: () => boolean
  name: () => string
  onCommit: (newName: string) => void
  onCancel: () => void
}) {
  const inputRef = ref<HTMLInputElement | null>(null)
  const draft = ref('')
  let done = false

  watch(opts.editing, (editing) => {
    if (!editing)
      return
    draft.value = opts.name()
    done = false
    nextTick(() => {
      const el = inputRef.value
      if (!el)
        return
      el.focus()
      // Select the base name (before the extension) like a desktop file manager.
      const dot = opts.name().lastIndexOf('.')
      if (dot > 0)
        el.setSelectionRange(0, dot)
      else
        el.select()
    })
  })

  function commit() {
    if (done)
      return
    done = true
    opts.onCommit(draft.value)
  }

  function cancel() {
    if (done)
      return
    done = true
    opts.onCancel()
  }

  return { inputRef, draft, commit, cancel }
}
