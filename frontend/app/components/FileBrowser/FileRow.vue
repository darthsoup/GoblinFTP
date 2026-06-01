<script setup lang="ts">
import type { FileInfo } from '~/types/api'

const props = defineProps<{
  file: FileInfo
  selected: boolean
  currentPath: string
}>()

const emit = defineEmits<{
  select: [name: string]
  navigate: [path: string]
  download: [path: string]
  contextmenu: [file: FileInfo, x: number, y: number]
}>()

const icon = computed(() =>
  props.file.isDir ? 'i-heroicons-folder' : 'i-heroicons-document',
)

const iconColor = computed(() =>
  props.file.isDir ? 'text-yellow-400' : 'text-gray-400',
)

function formatSize(bytes: number): string {
  if (props.file.isDir)
    return '—'
  if (bytes < 1024)
    return `${bytes} B`
  if (bytes < 1024 * 1024)
    return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024)
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString()
  }
  catch {
    return iso
  }
}

function handleClick() {
  if (props.file.isDir) {
    const path = `${props.currentPath.replace(/\/$/, '')}/${props.file.name}`
    emit('navigate', path)
  }
}

function handleDownload() {
  const path = `${props.currentPath.replace(/\/$/, '')}/${props.file.name}`
  emit('download', path)
}
</script>

<template>
  <tr
    class="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50 cursor-default"
    :class="{ 'bg-primary-50 dark:bg-primary-900/20': selected }"
    @click="handleClick"
    @contextmenu.prevent="emit('contextmenu', props.file, $event.clientX, $event.clientY)"
  >
    <td class="w-8 px-3 py-2">
      <input
        type="checkbox"
        :checked="selected"
        class="rounded"
        @click.stop
        @change="emit('select', file.name)"
      >
    </td>
    <td class="px-3 py-2">
      <div class="flex items-center gap-2">
        <UIcon :name="icon" class="w-4 h-4 shrink-0" :class="iconColor" />
        <span class="truncate">{{ file.name }}</span>
      </div>
    </td>
    <td class="px-3 py-2 text-right text-sm text-gray-500 dark:text-gray-400 w-24">
      {{ formatSize(file.size) }}
    </td>
    <td class="px-3 py-2 text-sm text-gray-500 dark:text-gray-400 w-40">
      {{ formatDate(file.modified) }}
    </td>
    <td class="px-3 py-2 font-mono text-xs text-gray-500 dark:text-gray-400 w-28 hidden sm:table-cell">
      {{ file.mode }}
    </td>
    <td class="px-3 py-2 w-16 text-center">
      <UButton
        v-if="!file.isDir"
        size="xs"
        color="neutral"
        variant="ghost"
        icon="i-heroicons-arrow-down-tray"
        @click.stop="handleDownload"
      />
    </td>
  </tr>
</template>
