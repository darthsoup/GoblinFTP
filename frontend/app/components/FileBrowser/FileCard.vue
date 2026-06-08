<script setup lang="ts">
import type { FileInfo } from '~/types/api'

const props = defineProps<{
  file: FileInfo
  selected: boolean
  currentPath: string
  editing: boolean
  isCut: boolean
  active: boolean
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

const { t, locale } = useI18n()
const settingsStore = useSettingsStore()

const iconDef = computed(() => getFileIcon(props.file))

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

// Secondary line: "size · date · perms" (no size for directories).
const meta = computed(() => {
  const parts: string[] = []
  if (!props.file.isDir)
    parts.push(formatFileSize(props.file.size, settingsStore.sizeFormat, locale.value))
  parts.push(formatFileDate(props.file.modified, settingsStore.dateFormat, locale.value))
  if (props.file.mode)
    parts.push(props.file.mode)
  return parts.join(' · ')
})

function handleClick() {
  if (props.file.isDir)
    emit('navigate', `${props.currentPath.replace(/\/$/, '')}/${props.file.name}`)
  else
    emit('preview')
}

function handleDownload() {
  emit('download', `${props.currentPath.replace(/\/$/, '')}/${props.file.name}`)
}
</script>

<template>
  <div
    role="listitem"
    class="group flex items-center gap-3 px-3 py-2.5 border-b border-muted cursor-pointer hover:bg-accented/40 transition-colors"
    :class="[
      selected ? 'bg-primary/10' : (active ? 'bg-accented/50' : ''),
      isCut ? 'opacity-50' : '',
    ]"
    :data-file-name="file.name"
    @click="handleClick"
  >
    <UCheckbox
      :model-value="selected"
      class="shrink-0"
      :aria-label="file.name"
      @click.stop
      @update:model-value="emit('select', file.name)"
    />
    <UIcon
      :name="iconDef.icon"
      class="size-6 shrink-0"
      :class="iconDef.primary ? 'text-primary' : (iconDef.color ? '' : 'text-dimmed')"
      :style="iconDef.color ? { color: iconDef.color } : undefined"
    />

    <div class="flex-1 min-w-0">
      <input
        v-if="editing"
        ref="inputRef"
        v-model="draft"
        class="w-full bg-default border border-primary rounded px-1.5 py-0.5 text-sm text-default outline-none focus:ring-1 focus:ring-primary"
        @click.stop
        @keydown.enter.prevent="commit"
        @keydown.escape.prevent="cancel"
        @blur="commit"
      >
      <div
        v-else
        class="truncate text-sm"
        :class="file.isDir ? 'font-semibold text-highlighted' : 'text-default'"
        @dblclick.stop="onNameDblClick"
      >
        {{ file.name }}
      </div>
      <div class="truncate text-xs font-mono text-muted">
        {{ meta }}
      </div>
    </div>

    <UButton
      v-if="!file.isDir"
      size="sm"
      color="neutral"
      variant="ghost"
      icon="i-lucide-download"
      class="shrink-0"
      :aria-label="t('context.download')"
      @click.stop="handleDownload"
    />
  </div>
</template>
