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
const filesStore = useFilesStore()
const settingsStore = useSettingsStore()

const iconDef = computed(() => getFileIcon(props.file))
const fullPath = computed(() => `${props.currentPath.replace(/\/$/, '')}/${props.file.name}`)

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

// Secondary line: "size · date" (date-only for directories).
const meta = computed(() => {
  const parts: string[] = []
  if (!props.file.isDir)
    parts.push(formatFileSize(props.file.size, settingsStore.sizeFormat, locale.value))
  parts.push(formatFileDate(props.file.modified, settingsStore.dateFormat, locale.value))
  return parts.join(' · ')
})

// ── Lazy thumbnail ────────────────────────────────────────────────────────────
const thumbEl = ref<HTMLElement | null>(null)
const thumbUrl = ref<string | null>(null)
const failed = ref(false)
const visible = ref(false)
let loading = false

const thumbnailEligible = computed(() =>
  settingsStore.gridThumbnails
  && !props.file.isDir
  && isImageFile(props.file.name)
  && props.file.size <= THUMBNAIL_MAX_BYTES,
)
const showThumb = computed(() => thumbnailEligible.value && !!thumbUrl.value && !failed.value)

// Observe once; load when the tile is both on-screen and currently eligible
// (so flipping the setting on reveals thumbnails for already-loaded tiles).
const { stop } = useIntersectionObserver(thumbEl, ([entry]) => {
  if (entry?.isIntersecting) {
    visible.value = true
    stop()
  }
})

watchEffect(() => {
  if (!visible.value || !thumbnailEligible.value || thumbUrl.value || loading)
    return
  loading = true
  filesStore.downloadUrl(fullPath.value)
    .then((url) => { thumbUrl.value = url })
    .catch(() => { failed.value = true })
    .finally(() => { loading = false })
})

function handleClick() {
  if (props.file.isDir)
    emit('navigate', fullPath.value)
  else
    emit('preview')
}

function handleDownload() {
  emit('download', fullPath.value)
}
</script>

<template>
  <div
    role="listitem"
    class="group relative flex flex-col cursor-pointer rounded-lg border border-default bg-elevated/50 p-2 transition-colors hover:bg-accented/40 hover:border-accented"
    :class="[
      selected ? 'ring-2 ring-primary border-primary bg-primary/5' : (active ? 'ring-1 ring-inset ring-accented' : ''),
      isCut ? 'opacity-50' : '',
    ]"
    :data-file-name="file.name"
    @click="handleClick"
  >
    <!-- Thumbnail / icon -->
    <div
      ref="thumbEl"
      class="relative aspect-square mb-2 rounded-md bg-muted/40 overflow-hidden flex items-center justify-center"
    >
      <img
        v-if="showThumb"
        :src="thumbUrl!"
        :alt="file.name"
        class="w-full h-full object-cover"
        @error="failed = true"
      >
      <UIcon
        v-else
        :name="iconDef.icon"
        class="size-10"
        :class="iconDef.primary ? 'text-primary' : (iconDef.color ? '' : 'text-dimmed')"
        :style="iconDef.color ? { color: iconDef.color } : undefined"
      />

      <!-- Selection checkbox (top-left), shown on hover or when selected -->
      <UCheckbox
        :model-value="selected"
        class="absolute top-1 left-1 transition-opacity"
        :class="selected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'"
        :aria-label="file.name"
        @click.stop
        @update:model-value="emit('select', file.name)"
      />

      <!-- Quick download (top-right, files only) -->
      <UButton
        v-if="!file.isDir"
        size="xs"
        color="neutral"
        variant="solid"
        icon="i-lucide-download"
        class="absolute top-1 right-1 opacity-0 group-hover:opacity-100 transition-opacity"
        :aria-label="t('context.download')"
        @click.stop="handleDownload"
      />
    </div>

    <!-- Name -->
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
      class="text-sm text-center truncate"
      :class="file.isDir ? 'font-semibold text-highlighted' : 'text-default'"
      :title="file.name"
      @dblclick.stop="onNameDblClick"
    >
      {{ file.name }}
    </div>

    <!-- Meta -->
    <div class="text-xs font-mono text-muted text-center truncate">
      {{ meta }}
    </div>
  </div>
</template>
