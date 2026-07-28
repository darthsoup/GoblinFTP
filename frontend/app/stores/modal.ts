import type { FileInfo, UploadConflict } from '~/types/api'
import { defineStore } from 'pinia'

export type ModalType = 'delete' | 'newFolder' | 'newFile' | 'properties' | 'settings' | 'shortcuts' | 'pasteConflict' | 'uploadConflict' | null

// How a paste should resolve name collisions, chosen via PasteConflictModal.
export type PasteChoice = 'overwrite' | 'append' | 'cancel'

// How a single upload resolves a name collision, chosen via UploadConflictModal.
export type UploadConflictAction = 'overwrite' | 'rename' | 'skip'

// Cancel drops the whole batch; resolve carries one action per conflicting path.
// applyToAll is remembered by the upload store so a conflict that only surfaces
// mid-transfer (the server changed under us) reuses the decision instead of
// interrupting again.
export type UploadConflictResolution
  = | { kind: 'cancel' }
    | { kind: 'resolve', decisions: Record<string, UploadConflictAction>, applyToAll?: UploadConflictAction }

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

  // ── Confirm dialog ─────────────────────────────────────────────────────────
  const confirmOptions = ref<ConfirmOptions | null>(null)
  let confirmResolver: ((result: ConfirmResult) => void) | null = null

  function confirm(options: ConfirmOptions): Promise<ConfirmResult> {
    // A pending confirm is superseded — resolve it as cancelled.
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

  // ── Paste conflict dialog ──────────────────────────────────────────────────
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

  // ── Upload conflict dialog ─────────────────────────────────────────────────
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
  }
})
