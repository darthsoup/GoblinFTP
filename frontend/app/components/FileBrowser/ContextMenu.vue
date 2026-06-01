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
  chmod: [file: FileInfo]
  properties: [file: FileInfo]
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
const chmodEnabled = computed(() => !authStore.systemVars?.connection.disableChmod)
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible && file"
      ref="menuRef"
      class="fixed z-50 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg py-1 min-w-40"
      :style="menuStyle"
    >
      <button
        class="w-full text-left px-4 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800 flex items-center gap-2"
        @click="emit('download', file); emit('close')"
      >
        <UIcon name="i-heroicons-arrow-down-tray" class="w-4 h-4" />
        {{ t('context.download') }}
      </button>
      <div class="border-t border-gray-100 dark:border-gray-800 my-1" />
      <button
        class="w-full text-left px-4 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800 flex items-center gap-2"
        @click="emit('rename', file); emit('close')"
      >
        <UIcon name="i-heroicons-pencil" class="w-4 h-4" />
        {{ t('context.rename') }}
      </button>
      <button
        v-if="chmodEnabled && !file.isDir"
        class="w-full text-left px-4 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800 flex items-center gap-2"
        @click="emit('chmod', file); emit('close')"
      >
        <UIcon name="i-heroicons-lock-closed" class="w-4 h-4" />
        {{ t('context.permissions') }}
      </button>
      <button
        class="w-full text-left px-4 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800 flex items-center gap-2"
        @click="emit('properties', file); emit('close')"
      >
        <UIcon name="i-heroicons-information-circle" class="w-4 h-4" />
        {{ t('context.properties') }}
      </button>
      <div class="border-t border-gray-100 dark:border-gray-800 my-1" />
      <button
        class="w-full text-left px-4 py-1.5 text-sm text-red-500 hover:bg-gray-100 dark:hover:bg-gray-800 flex items-center gap-2"
        @click="emit('delete', file); emit('close')"
      >
        <UIcon name="i-heroicons-trash" class="w-4 h-4" />
        {{ t('context.delete') }}
      </button>
    </div>
  </Teleport>
</template>
