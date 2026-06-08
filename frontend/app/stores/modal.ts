import type { FileInfo } from '~/types/api'
import { defineStore } from 'pinia'

export type ModalType = 'delete' | 'newFolder' | 'newFile' | 'properties' | 'settings' | 'shortcuts' | 'pasteConflict' | null

// How a paste should resolve name collisions, chosen via PasteConflictModal.
export type PasteChoice = 'overwrite' | 'append' | 'cancel'

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

  return { active, context, open, close, confirmOptions, confirm, resolveConfirm, pasteConflicts, pasteConflict, resolvePaste }
})
