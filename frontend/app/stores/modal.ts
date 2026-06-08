import type { FileInfo } from '~/types/api'
import { defineStore } from 'pinia'

export type ModalType = 'rename' | 'delete' | 'newFolder' | 'newFile' | 'properties' | 'settings' | null

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

  return { active, context, open, close, confirmOptions, confirm, resolveConfirm }
})
