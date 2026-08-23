import type { FileInfo, UploadConflict } from '~/types/api'
import { defineStore } from 'pinia'

export type ModalType = 'delete' | 'newFolder' | 'newFile' | 'properties' | 'settings' | 'shortcuts' | 'pasteConflict' | 'uploadConflict' | 'editorConflict' | null

// How a paste should resolve name collisions, chosen via PasteConflictModal.
export type PasteChoice = 'overwrite' | 'append' | 'cancel'

// How a single upload resolves a name collision, chosen via UploadConflictModal.
export type UploadConflictAction = 'overwrite' | 'rename' | 'skip'

// Cancel drops the whole batch; resolve carries one action per path. applyToAll
// is reused for conflicts surfacing mid-transfer, instead of prompting again.
export type UploadConflictResolution
  = | { kind: 'cancel' }
    | { kind: 'resolve', decisions: Record<string, UploadConflictAction>, applyToAll?: UploadConflictAction }

// 'modified': the save collided with a newer server copy. 'deleted': the file is
// gone. Overwrite replaces, reload discards the buffer, cancel pauses autosave.
export type EditorConflictKind = 'modified' | 'deleted'
export type EditorConflictChoice = 'overwrite' | 'reload' | 'cancel'

// size/modified describe the version the user opened, which is the honest thing
// to show: reading the server's current metadata would need another round trip.
export interface EditorConflictInfo {
  name: string
  kind: EditorConflictKind
  size?: number
  modified?: string
}

export interface ModalContext {
  file?: FileInfo
  files?: string[] // absolute paths for bulk delete
}

// Promise-based confirm dialog (rendered by ConfirmModal in the layout). Callers
// `await modalStore.confirm({...})` and branch on the result.
export interface ConfirmOptions {
  title: string
  message: string
  confirmLabel: string
  cancelLabel: string
  saveLabel?: string // when set, a third "Save" button is shown
  confirmColor?: 'primary' | 'error'
}
export type ConfirmResult = 'confirm' | 'save' | 'cancel'

export const useModalStore = defineStore('modal', () => {
  const active = ref<ModalType>(null)
  const context = ref<ModalContext>({})

  function open(type: Exclude<ModalType, null>, ctx: ModalContext = {}) {
    active.value = type
    context.value = ctx
  }

  function close() {
    active.value = null
    context.value = {}
  }

  const confirmOptions = ref<ConfirmOptions | null>(null)
  let confirmResolver: ((result: ConfirmResult) => void) | null = null

  function confirm(options: ConfirmOptions): Promise<ConfirmResult> {
    // A pending confirm is superseded: resolve it as cancelled.
    confirmResolver?.('cancel')
    confirmOptions.value = options
    return new Promise((resolve) => {
      confirmResolver = resolve
    })
  }

  function resolveConfirm(result: ConfirmResult) {
    confirmOptions.value = null
    const resolve = confirmResolver
    confirmResolver = null
    resolve?.(result)
  }

  // Mirrors confirm(): the files store `await`s a PasteChoice while
  // PasteConflictModal renders the conflicting names + the three buttons.
  const pasteConflicts = ref<string[]>([])
  let pasteResolver: ((choice: PasteChoice) => void) | null = null

  function pasteConflict(names: string[]): Promise<PasteChoice> {
    pasteResolver?.('cancel') // supersede any pending one
    pasteConflicts.value = names
    active.value = 'pasteConflict'
    return new Promise((resolve) => {
      pasteResolver = resolve
    })
  }

  function resolvePaste(choice: PasteChoice) {
    pasteConflicts.value = []
    if (active.value === 'pasteConflict')
      active.value = null
    const resolve = pasteResolver
    pasteResolver = null
    resolve?.(choice)
  }

  // Kept separate from pasteConflict: uploads resolve per file (and can skip),
  // which a single batch-wide PasteChoice cannot express.
  const uploadConflicts = ref<UploadConflict[]>([])
  let uploadResolver: ((result: UploadConflictResolution) => void) | null = null

  function uploadConflict(entries: UploadConflict[]): Promise<UploadConflictResolution> {
    uploadResolver?.({ kind: 'cancel' }) // supersede any pending one
    uploadConflicts.value = entries
    active.value = 'uploadConflict'
    return new Promise((resolve) => {
      uploadResolver = resolve
    })
  }

  function resolveUploadConflict(result: UploadConflictResolution) {
    uploadConflicts.value = []
    if (active.value === 'uploadConflict')
      active.value = null
    const resolve = uploadResolver
    uploadResolver = null
    resolve?.(result)
  }

  // Its own dialog rather than confirm(): three outcomes, and the payload
  // carries the baseline the user opened so the copy can show it.
  const editorConflictInfo = ref<EditorConflictInfo | null>(null)
  let editorResolver: ((choice: EditorConflictChoice) => void) | null = null

  function editorConflict(info: EditorConflictInfo): Promise<EditorConflictChoice> {
    editorResolver?.('cancel') // supersede any pending one
    editorConflictInfo.value = info
    active.value = 'editorConflict'
    return new Promise((resolve) => {
      editorResolver = resolve
    })
  }

  function resolveEditorConflict(choice: EditorConflictChoice) {
    editorConflictInfo.value = null
    if (active.value === 'editorConflict')
      active.value = null
    const resolve = editorResolver
    editorResolver = null
    resolve?.(choice)
  }

  return {
    active,
    context,
    open,
    close,
    confirmOptions,
    confirm,
    resolveConfirm,
    pasteConflicts,
    pasteConflict,
    resolvePaste,
    uploadConflicts,
    uploadConflict,
    resolveUploadConflict,
    editorConflictInfo,
    editorConflict,
    resolveEditorConflict,
  }
})
