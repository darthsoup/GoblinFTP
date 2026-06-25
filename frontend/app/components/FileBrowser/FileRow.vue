<script setup lang="ts">
import type { FileInfo } from '~/types/api'

const props = defineProps<{
  file: FileInfo
  selected: boolean
  currentPath: string
  editing: boolean
  isCut: boolean
  active: boolean
  compact: boolean
  showPermissions: boolean
  index: number
}>()

const emit = defineEmits<{
  select: [name: string]
  navigate: [path: string]
  download: [path: string]
  commitRename: [newName: string]
  cancelRename: []
  requestRename: []
  preview: []
}>()

const { locale } = useI18n()
const settingsStore = useSettingsStore()

const iconDef = computed(() => getFileIcon(props.file))

// Inline rename behaviour is shared with FileCard.
const { inputRef, draft, commit, cancel } = useInlineRename({
  editing: () => props.editing,
  name: () => props.file.name,
  onCommit: name => emit('commitRename', name),
  onCancel: () => emit('cancelRename'),
})

function onNameDblClick() {
  if (!props.file.isDir)
    emit('requestRename')
}

function formatSize(bytes: number): string {
  return formatFileSize(bytes, settingsStore.sizeFormat, locale.value)
}

function formatDate(iso: string): string {
  return formatFileDate(iso, settingsStore.dateFormat, locale.value)
}

function handleClick() {
  if (props.file.isDir) {
    const path = `${props.currentPath.replace(/\/$/, '')}/${props.file.name}`
    emit('navigate', path)
  }
  else {
    emit('preview')
  }
}

function handleDownload() {
  const path = `${props.currentPath.replace(/\/$/, '')}/${props.file.name}`
  emit('download', path)
}
</script>

<template>
  <tr
    class="file-row group border-b border-muted cursor-pointer hover:bg-accented/40 transition-colors text-[13px]"
    :class="[
      compact ? 'h-9' : 'h-11',
      selected ? 'bg-primary/10' : (active ? 'bg-accented/50' : 'even:bg-elevated/40'),
      isCut ? 'opacity-50' : '',
    ]"
    :style="{ '--row-i': index }"
    :data-file-name="file.name"
    @click="handleClick"
  >
    <td class="w-10 px-3">
      <UCheckbox
        :model-value="selected"
        class="justify-center transition-opacity"
        :class="selected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'"
        :aria-label="file.name"
        @click.stop
        @update:model-value="emit('select', file.name)"
      />
    </td>
    <td class="px-3 truncate max-w-0">
      <input
        v-if="editing"
        ref="inputRef"
        v-model="draft"
        class="w-full bg-default border border-primary rounded px-1.5 py-0.5 text-default outline-none focus:ring-1 focus:ring-primary"
        @click.stop
        @keydown.enter.prevent="commit"
        @keydown.escape.prevent="cancel"
        @blur="commit"
      >
      <div v-else class="flex items-center gap-2.5 min-w-0">
        <UIcon
          :name="iconDef.icon"
          class="size-5 shrink-0"
          :class="iconDef.primary ? 'text-primary' : (iconDef.color ? '' : 'text-dimmed')"
          :style="iconDef.color ? { color: iconDef.color } : undefined"
        />
        <span
          class="truncate"
          :class="file.isDir ? 'font-semibold text-highlighted' : 'text-default'"
          @dblclick.stop="onNameDblClick"
        >{{ file.name }}</span>
      </div>
    </td>
    <td class="w-24 px-3 text-right text-muted whitespace-nowrap hidden sm:table-cell">
      <span v-if="file.isDir" class="text-dimmed/50">–</span>
      <span v-else>{{ formatSize(file.size) }}</span>
    </td>
    <td class="w-40 px-3 text-right text-muted whitespace-nowrap hidden md:table-cell">
      {{ formatDate(file.modified) }}
    </td>
    <td v-if="showPermissions" class="w-28 px-3 text-center text-dimmed text-xs hidden sm:table-cell whitespace-nowrap">
      <span v-if="file.mode">{{ file.mode }}</span>
      <span v-else class="text-dimmed/50">–</span>
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

<style scoped>
/* Staggered reveal on directory load; delay capped so large listings settle fast. */
.file-row {
  animation: row-in 0.26s ease backwards;
  animation-delay: min(calc(var(--row-i) * 12ms), 280ms);
}
@keyframes row-in {
  from {
    opacity: 0;
    transform: translateY(3px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
@media (prefers-reduced-motion: reduce) {
  .file-row {
    animation: none;
  }
}
</style>
