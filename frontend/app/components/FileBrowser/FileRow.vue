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

const { locale } = useI18n()

const iconDef = computed(() => getFileIcon(props.file))

function formatSize(bytes: number): string {
  if (props.file.isDir)
    return '--'
  if (bytes < 1024)
    return `${bytes} B`
  if (bytes < 1024 * 1024)
    return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024)
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime()))
    return iso
  const sameYear = d.getFullYear() === new Date().getFullYear()
  return new Intl.DateTimeFormat(locale.value, sameYear
    ? { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }
    : { year: 'numeric', month: 'short', day: '2-digit' }).format(d)
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
    class="group h-11 border-b border-muted hover:bg-accented/40 transition-colors text-[13px]"
    :class="[
      selected ? 'bg-primary/10' : 'even:bg-elevated/40',
      file.isDir ? 'cursor-pointer' : 'cursor-default',
    ]"
    @click="handleClick"
    @contextmenu.prevent="emit('contextmenu', props.file, $event.clientX, $event.clientY)"
  >
    <td class="w-10 px-3 text-center">
      <input
        type="checkbox"
        :checked="selected"
        class="rounded align-middle transition-opacity"
        :class="selected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'"
        @click.stop
        @change="emit('select', file.name)"
      >
    </td>
    <td class="w-12 px-2 text-center">
      <UIcon
        :name="iconDef.icon"
        class="size-5 align-middle"
        :class="iconDef.primary ? 'text-primary' : (iconDef.color ? '' : 'text-dimmed')"
        :style="iconDef.color ? { color: iconDef.color } : undefined"
      />
    </td>
    <td class="px-3 truncate max-w-0">
      <span :class="file.isDir ? 'font-semibold text-highlighted' : 'text-default'">{{ file.name }}</span>
    </td>
    <td class="w-24 px-3 text-right text-muted whitespace-nowrap">
      {{ formatSize(file.size) }}
    </td>
    <td class="w-40 px-3 text-right text-muted whitespace-nowrap">
      {{ formatDate(file.modified) }}
    </td>
    <td class="w-28 px-3 text-center text-dimmed text-xs hidden sm:table-cell whitespace-nowrap">
      {{ file.mode || '--' }}
    </td>
    <td class="w-14 px-2 text-center">
      <UButton
        v-if="!file.isDir"
        size="xs"
        color="neutral"
        variant="ghost"
        icon="i-lucide-download"
        class="opacity-0 group-hover:opacity-100 transition-opacity"
        @click.stop="handleDownload"
      />
    </td>
  </tr>
</template>
