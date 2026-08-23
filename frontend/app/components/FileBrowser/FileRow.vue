<template>
  <tr
    class="file-row group border-b border-muted cursor-pointer hover:bg-accented/40 transition-colors text-sm outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary"
    :class="[
      compact ? 'h-9' : 'h-12',
      selected ? 'bg-primary/10' : (active ? 'bg-accented/50' : 'even:bg-elevated/40'),
      isCut ? 'opacity-50' : '',
    ]"
    :style="{ '--row-i': index }"
    :data-file-name="file.name"
    :tabindex="index === 0 ? 0 : -1"
    :aria-selected="selected"
    :aria-label="rowLabel"
    @click="handleClick"
    @keydown="onKeydown"
  >
    <td class="w-10 px-4">
      <UCheckbox
        :model-value="selected"
        size="md"
        class="justify-center transition-opacity"
        :class="selected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'"
        :aria-label="t('files.selectItem', { name: file.name })"
        tabindex="-1"
        @click.stop
        @update:model-value="emit('select', file.name)"
      />
    </td>
    <td class="px-4 truncate max-w-0">
      <input
        v-if="editing"
        ref="inputRef"
        v-model="draft"
        class="w-full bg-default border border-primary rounded px-1.5 py-0.5 text-default outline-none focus:ring-1 focus:ring-primary"
        :aria-label="t('files.renameItem', { name: file.name })"
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
    <td class="w-24 px-4 text-right text-muted whitespace-nowrap hidden sm:table-cell">
      <span v-if="file.isDir" class="text-dimmed/50">-</span>
      <span v-else>{{ formatSize(file.size) }}</span>
    </td>
    <td class="w-40 px-4 text-right text-muted whitespace-nowrap hidden md:table-cell">
      {{ formatDate(file.modified) }}
    </td>
    <td v-if="showPermissions" class="w-28 px-4 text-center text-dimmed text-xs hidden sm:table-cell whitespace-nowrap">
      <span v-if="file.mode">{{ file.mode }}</span>
      <span v-else class="text-dimmed/50">-</span>
    </td>
    <td class="w-14 px-2 text-center">
      <UDropdownMenu :items="menuItems" :content="{ align: 'end' }">
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-ellipsis-vertical"
          :aria-label="t('context.actions')"
          class="opacity-0 group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100 transition-opacity"
          @click.stop
        />
      </UDropdownMenu>
    </td>
  </tr>
</template>

<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'
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
  // Action menu for this row, built by the parent so the "⋮" dropdown and the
  // right-click context menu share one source of truth.
  menuItems: DropdownMenuItem[][]
}>()

const emit = defineEmits<{
  select: [name: string]
  navigate: [path: string]
  commitRename: [newName: string]
  cancelRename: []
  requestRename: []
  preview: []
  focusMove: [delta: number | 'first' | 'last']
}>()

const { t, locale } = useI18n()
const settingsStore = useSettingsStore()

const iconDef = computed(() => getFileIcon(props.file))

// Screen readers announce the row as a whole; without this they read the cells
// with no indication of what the row IS.
const rowLabel = computed(() =>
  props.file.isDir
    ? t('files.folderNamed', { name: props.file.name })
    : t('files.fileNamed', { name: props.file.name }),
)

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

// Roving tabindex: one row is in the tab order, arrows move focus between them.
// A bare <tr @click> left the whole file manager keyboard-inoperable (WCAG 2.1.1).
function onKeydown(e: KeyboardEvent) {
  switch (e.key) {
    case 'Enter':
      e.preventDefault()
      handleClick()
      break
    case ' ':
      e.preventDefault()
      emit('select', props.file.name)
      break
    case 'ArrowDown':
      e.preventDefault()
      emit('focusMove', 1)
      break
    case 'ArrowUp':
      e.preventDefault()
      emit('focusMove', -1)
      break
    case 'Home':
      e.preventDefault()
      emit('focusMove', 'first')
      break
    case 'End':
      e.preventDefault()
      emit('focusMove', 'last')
      break
    case 'F2':
      if (!props.file.isDir) {
        e.preventDefault()
        emit('requestRename')
      }
      break
  }
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
</script>

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
