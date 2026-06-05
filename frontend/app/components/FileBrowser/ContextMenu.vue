<!-- frontend/app/components/FileBrowser/ContextMenu.vue -->
<script setup lang="ts">
import type { FileInfo } from '~/types/api'

const props = defineProps<{
  file: FileInfo | null
  x: number
  y: number
  visible: boolean
}>()

const emit = defineEmits<{
  close: []
  download: [file: FileInfo]
  rename: [file: FileInfo]
  delete: [file: FileInfo]
  properties: [file: FileInfo]
  edit: [file: FileInfo]
}>()

const { t } = useI18n()

const menuRef = ref<HTMLElement | null>(null)
const menuStyle = ref<Record<string, string>>({})

watch(() => props.visible, async (v) => {
  if (!v)
    return
  await nextTick()
  if (!menuRef.value)
    return
  const w = menuRef.value.offsetWidth
  const h = menuRef.value.offsetHeight
  const vw = window.innerWidth
  const vh = window.innerHeight
  let left = props.x + w > vw ? props.x - w : props.x
  let top = props.y + h > vh ? props.y - h : props.y
  left = Math.max(0, Math.min(left, vw - w))
  top = Math.max(0, Math.min(top, vh - h))
  menuStyle.value = { left: `${left}px`, top: `${top}px` }
})

function onClickOutside(e: MouseEvent) {
  if (menuRef.value && !menuRef.value.contains(e.target as Node)) {
    emit('close')
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape')
    emit('close')
}

onMounted(() => {
  document.addEventListener('mousedown', onClickOutside)
  document.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  document.removeEventListener('mousedown', onClickOutside)
  document.removeEventListener('keydown', onKeydown)
})

const authStore = useAuthStore()
const editEnabled = computed(() => {
  const editor = authStore.systemVars?.editor
  if (!editor || editor.disabled)
    return (_file: FileInfo) => false
  return (file: FileInfo) => {
    if (file.isDir)
      return false
    const ext = file.name.split('.').pop()?.toLowerCase() ?? ''
    return editor.allowedExtensions.some(a => a.toLowerCase() === ext)
  }
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible && file"
      ref="menuRef"
      class="fixed z-50 min-w-48 bg-accented border border-accented rounded-md shadow-lg py-1 px-1 text-sm text-default"
      :style="menuStyle"
    >
      <button
        class="w-full text-left px-2.5 py-1.5 rounded-sm hover:bg-neutral-600/50 transition-colors flex items-center gap-2"
        @click="emit('download', file); emit('close')"
      >
        <UIcon name="i-lucide-download" class="size-4 text-muted" />
        {{ t('context.download') }}
      </button>
      <div class="border-t border-accented/60 my-1 mx-1" />
      <button
        class="w-full text-left px-2.5 py-1.5 rounded-sm hover:bg-neutral-600/50 transition-colors flex items-center gap-2"
        @click="emit('rename', file); emit('close')"
      >
        <UIcon name="i-lucide-pencil-line" class="size-4 text-muted" />
        {{ t('context.rename') }}
      </button>
      <button
        v-if="file && !file.isDir && editEnabled(file)"
        class="w-full text-left px-2.5 py-1.5 rounded-sm hover:bg-neutral-600/50 transition-colors flex items-center gap-2"
        @click="emit('edit', file); emit('close')"
      >
        <UIcon name="i-lucide-pencil" class="size-4 text-muted" />
        {{ authStore.systemVars?.editor?.viewOnly ? t('context.view') : t('context.edit') }}
      </button>
      <button
        class="w-full text-left px-2.5 py-1.5 rounded-sm hover:bg-neutral-600/50 transition-colors flex items-center gap-2"
        @click="emit('properties', file); emit('close')"
      >
        <UIcon name="i-lucide-info" class="size-4 text-muted" />
        {{ t('context.properties') }}
      </button>
      <div class="border-t border-accented/60 my-1 mx-1" />
      <button
        class="w-full text-left px-2.5 py-1.5 rounded-sm text-error hover:bg-error/10 transition-colors flex items-center gap-2"
        @click="emit('delete', file); emit('close')"
      >
        <UIcon name="i-lucide-trash-2" class="size-4" />
        {{ t('context.delete') }}
      </button>
    </div>
  </Teleport>
</template>
