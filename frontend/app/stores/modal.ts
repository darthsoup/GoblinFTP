import type { FileInfo } from '~/types/api'
import { defineStore } from 'pinia'

export type ModalType = 'rename' | 'delete' | 'chmod' | 'newFolder' | 'newFile' | 'properties' | null

export interface ModalContext {
  file?: FileInfo
  files?: string[]  // absolute paths for bulk delete
}

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

  return { active, context, open, close }
})
