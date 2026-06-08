<script setup lang="ts">
import type { ButtonProps, ContextMenuItem } from '@nuxt/ui'
import type { FileInfo } from '~/types/api'
import { ApiError } from '~/types/api'

const filesStore = useFilesStore()
const modalStore = useModalStore()
const authStore = useAuthStore()
const settingsStore = useSettingsStore()
const notify = useNotify()
const { t } = useI18n()

async function onDownload(path: string) {
  try {
    await filesStore.downloadFile(path)
  }
  catch (e) {
    notify.error(e instanceof ApiError ? e.message : t('toast.downloadFailed'))
  }
}

// Copy/cut act on the whole selection when the right-clicked file is part of it,
// otherwise just that file (desktop file-manager behaviour).
function clipboardNames(file: FileInfo): string[] {
  return filesStore.selected.has(file.name) ? [...filesStore.selected] : [file.name]
}

const runPaste = usePaste()

// Inline rename: commit (always exits edit mode; re-initiate to retry on error).
async function onCommitRename(file: FileInfo, newName: string) {
  const trimmed = newName.trim()
  const dir = filesStore.currentPath.replace(/\/$/, '')
  if (!trimmed || trimmed === file.name) {
    filesStore.cancelRename()
    return
  }
  try {
    await filesStore.rename(`${dir}/${file.name}`, `${dir}/${trimmed}`)
    notify.success(t('toast.renamed', { name: trimmed }))
  }
  catch (e) {
    notify.error(e instanceof ApiError ? e.message : t('error.operationFailed'))
  }
  finally {
    filesStore.cancelRename()
  }
}

type SortKey = 'name' | 'size' | 'modified'
// Tri-state: ascending → descending → off (null = server order)
const sortKey = ref<SortKey | null>('name')
const sortAsc = ref(true)

const filter = ref('')
watch(() => filesStore.currentPath, () => {
  filter.value = ''
})

function toggleSort(key: SortKey) {
  if (sortKey.value !== key) {
    sortKey.value = key
    sortAsc.value = true
  }
  else if (sortAsc.value) {
    sortAsc.value = false
  }
  else {
    sortKey.value = null
  }
}

function sortIcon(key: SortKey): string {
  if (sortKey.value !== key)
    return 'i-lucide-chevrons-up-down'
  return sortAsc.value ? 'i-lucide-arrow-up-narrow-wide' : 'i-lucide-arrow-down-wide-narrow'
}

function ariaSort(key: SortKey): 'ascending' | 'descending' | 'none' {
  if (sortKey.value !== key)
    return 'none'
  return sortAsc.value ? 'ascending' : 'descending'
}

const sortedFiles = computed(() => {
  const key = sortKey.value
  if (!key)
    return filesStore.files
  const arr = [...filesStore.files]
  // Directories always first
  arr.sort((a, b) => {
    if (a.isDir !== b.isDir)
      return a.isDir ? -1 : 1
    const av = a[key]
    const bv = b[key]
    const cmp = typeof av === 'string' ? av.localeCompare(bv as string) : (av as number) - (bv as number)
    return sortAsc.value ? cmp : -cmp
  })
  return arr
})

const visibleFiles = computed(() => {
  let arr = sortedFiles.value
  if (!settingsStore.showDotfiles)
    arr = arr.filter(f => !f.name.startsWith('.'))
  const q = filter.value.trim().toLowerCase()
  if (!q)
    return arr
  return arr.filter(f => f.name.toLowerCase().includes(q))
})

// Browser-only keyboard shortcuts (select-all matches the visible/filtered set).
useFileBrowserShortcuts(() => visibleFiles.value.map(f => f.name))

// ── Empty state ───────────────────────────────────────────────────────────────
const emptyActions = computed<ButtonProps[]>(() => {
  if (filter.value) {
    return [{
      label: t('files.clearFilter'),
      icon: 'i-lucide-x',
      color: 'neutral',
      variant: 'subtle',
      onClick: () => {
        filter.value = ''
      },
    }]
  }
  return [
    { label: t('toolbar.newFolder'), icon: 'i-lucide-folder-plus', onClick: () => modalStore.open('newFolder') },
    { label: t('toolbar.newFile'), icon: 'i-lucide-file-plus', color: 'neutral', variant: 'subtle', onClick: () => modalStore.open('newFile') },
  ]
})

// ── Selection ─────────────────────────────────────────────────────────────────
const allSelected = computed(() =>
  visibleFiles.value.length > 0 && visibleFiles.value.every(f => filesStore.selected.has(f.name)),
)

const headerChecked = computed<boolean | 'indeterminate'>(() => {
  if (allSelected.value)
    return true
  return visibleFiles.value.some(f => filesStore.selected.has(f.name)) ? 'indeterminate' : false
})

function toggleSelectAll() {
  if (allSelected.value)
    filesStore.clearSelection()
  else
    filesStore.setSelection(visibleFiles.value.map(f => f.name))
}

// Names dimmed as "pending move" — only the cut items still in their source dir.
const cutNames = computed(() => {
  const cb = filesStore.clipboard
  if (!cb || cb.mode !== 'cut' || cb.sourcePath !== filesStore.currentPath.replace(/\/$/, ''))
    return new Set<string>()
  return new Set(cb.names)
})

// ── Preview panel ─────────────────────────────────────────────────────────────
// Derive the previewed file from the store so it auto-clears when the entry
// disappears after a refresh; reset on navigation.
const previewName = ref<string | null>(null)
const previewFile = computed(() =>
  previewName.value ? filesStore.files.find(f => f.name === previewName.value) ?? null : null,
)
watch(() => filesStore.currentPath, () => {
  previewName.value = null
})

// Table vs. stacked-cards layout (user toggle; defaults by viewport width).
const viewMode = computed(() => settingsStore.fileViewMode)

// ── Context menu ──────────────────────────────────────────────────────────────
const menuFile = ref<FileInfo | null>(null)

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

const menuItems = computed<ContextMenuItem[][]>(() => {
  const file = menuFile.value
  if (!file)
    return []
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const path = `${dir}/${file.name}`

  const middle: ContextMenuItem[] = [
    { label: t('context.rename'), icon: 'i-lucide-pencil-line', onSelect: () => filesStore.startRename(file.name) },
  ]
  if (editEnabled.value(file)) {
    middle.push({
      label: authStore.systemVars?.editor?.viewOnly ? t('context.view') : t('context.edit'),
      icon: 'i-lucide-pencil',
      onSelect: () => navigateTo({ path: '/edit', query: { path } }),
    })
  }
  middle.push({ label: t('context.properties'), icon: 'i-lucide-info', onSelect: () => modalStore.open('properties', { file }) })

  const clipboard: ContextMenuItem[] = [
    { label: t('context.copy'), icon: 'i-lucide-copy', onSelect: () => filesStore.copyToClipboard(clipboardNames(file)) },
    { label: t('context.cut'), icon: 'i-lucide-scissors', onSelect: () => filesStore.cutToClipboard(clipboardNames(file)) },
  ]
  if (filesStore.clipboard)
    clipboard.push({ label: t('context.paste'), icon: 'i-lucide-clipboard-paste', onSelect: runPaste })

  return [
    [{ label: t('context.download'), icon: 'i-lucide-download', onSelect: () => onDownload(path) }],
    middle,
    clipboard,
    [{ label: t('context.delete'), icon: 'i-lucide-trash-2', color: 'error', onSelect: () => modalStore.open('delete', { file }) }],
  ]
})

// Capture-phase: resolve the right-clicked row/card before Reka's trigger opens
// the menu; on empty space, stop the event so the browser menu shows instead.
// `[data-file-name]` matches both the table <tr> and the cards <div>.
function onAreaContextMenu(e: MouseEvent) {
  const row = (e.target as HTMLElement).closest<HTMLElement>('[data-file-name]')
  const file = row ? visibleFiles.value.find(f => f.name === row.dataset.fileName) : undefined
  if (!file) {
    e.stopPropagation()
    return
  }
  menuFile.value = file
}

// ── Drag & drop upload ────────────────────────────────────────────────────────
const uploadStore = useUploadStore()
const isDragOver = ref(false)
let dragCounter = 0 // counter to handle child element enter/leave events

function onDragEnter(e: DragEvent) {
  if (!e.dataTransfer?.types.includes('Files'))
    return
  dragCounter++
  isDragOver.value = true
}

function onDragLeave() {
  dragCounter--
  if (dragCounter <= 0) {
    dragCounter = 0
    isDragOver.value = false
  }
}

function onDrop(e: DragEvent) {
  dragCounter = 0
  isDragOver.value = false
  const files = e.dataTransfer?.files
  if (files && files.length > 0)
    uploadStore.addFiles(files, filesStore.currentPath)
}
</script>

<template>
  <div
    class="relative flex flex-col flex-1 min-h-0 overflow-hidden"
    @dragenter.prevent="onDragEnter"
    @dragover.prevent
    @dragleave="onDragLeave"
    @drop.prevent="onDrop"
  >
    <Transition name="fade">
      <div
        v-if="isDragOver"
        class="absolute inset-0 z-10 m-3 flex items-center justify-center rounded-lg border-2 border-dashed border-primary bg-default/90 backdrop-blur-sm pointer-events-none"
      >
        <div class="flex flex-col items-center gap-2 text-primary">
          <UIcon name="i-lucide-cloud-upload" class="size-12" />
          <span class="text-lg font-semibold">{{ t('files.dropToUpload') }}</span>
        </div>
      </div>
    </Transition>

    <FileToolbar v-model:filter="filter" />

    <div class="relative flex flex-1 min-h-0">
      <UContextMenu :items="menuItems">
        <div class="flex-1 min-w-0 overflow-auto" @contextmenu.capture="onAreaContextMenu">
          <!-- Loading / error / empty — shared by both views -->
          <div v-if="filesStore.loading" class="py-12 text-center text-muted font-mono text-sm">
            <UIcon name="i-lucide-loader-circle" class="size-5 animate-spin inline-block mr-2 align-middle text-primary" />
            {{ t('files.loading') }}
          </div>

          <div v-else-if="filesStore.error" class="py-8 flex flex-col items-center gap-3">
            <p class="text-error font-mono text-sm text-center px-4">
              <UIcon name="i-lucide-triangle-alert" class="size-5 inline-block mr-2 align-middle" />
              {{ filesStore.error }}
            </p>
            <UButton
              size="sm"
              color="neutral"
              variant="subtle"
              icon="i-lucide-refresh-cw"
              :loading="filesStore.loading"
              @click="filesStore.list()"
            >
              {{ t('files.retry') }}
            </UButton>
          </div>

          <div v-else-if="visibleFiles.length === 0" class="py-10">
            <UEmpty
              variant="naked"
              :icon="filter ? 'i-lucide-search-x' : 'i-lucide-folder-open'"
              :title="filter ? t('files.noMatches') : t('files.empty')"
              :description="filter ? undefined : t('files.dropToUpload')"
              :actions="emptyActions"
              :ui="{ title: 'font-mono', description: 'font-mono text-dimmed' }"
            />
          </div>

          <!-- Table view -->
          <table v-else-if="viewMode === 'table'" class="w-full text-left border-collapse">
            <thead class="sticky top-0 z-[5] bg-muted/95 backdrop-blur label-caps text-muted">
              <tr class="border-b border-default shadow-sm">
                <th class="w-10 px-3 py-2.5">
                  <UCheckbox
                    :model-value="headerChecked"
                    class="justify-center"
                    :aria-label="allSelected ? t('toolbar.deselectAll') : t('toolbar.selectAll')"
                    @update:model-value="toggleSelectAll"
                  />
                </th>
                <th class="w-12 px-2 py-2.5 text-center font-bold">
                  {{ t('files.type') }}
                </th>
                <th class="px-3 py-2.5 cursor-pointer hover:text-primary font-bold transition-colors" :aria-sort="ariaSort('name')" @click="toggleSort('name')">
                  {{ t('files.name') }}
                  <UIcon :name="sortIcon('name')" class="size-3 inline-block ml-1 align-middle" :class="sortKey === 'name' ? 'text-primary' : 'text-dimmed'" />
                </th>
                <th class="w-24 px-3 py-2.5 text-right cursor-pointer hover:text-primary font-bold transition-colors hidden sm:table-cell" :aria-sort="ariaSort('size')" @click="toggleSort('size')">
                  {{ t('files.size') }}
                  <UIcon :name="sortIcon('size')" class="size-3 inline-block ml-1 align-middle" :class="sortKey === 'size' ? 'text-primary' : 'text-dimmed'" />
                </th>
                <th class="w-40 px-3 py-2.5 text-right cursor-pointer hover:text-primary font-bold transition-colors hidden md:table-cell" :aria-sort="ariaSort('modified')" @click="toggleSort('modified')">
                  {{ t('files.modified') }}
                  <UIcon :name="sortIcon('modified')" class="size-3 inline-block ml-1 align-middle" :class="sortKey === 'modified' ? 'text-primary' : 'text-dimmed'" />
                </th>
                <th class="w-28 px-3 py-2.5 text-center font-bold hidden sm:table-cell">
                  {{ t('files.permissions') }}
                </th>
                <th class="w-14" />
              </tr>
            </thead>

            <tbody class="font-mono">
              <FileRow
                v-for="file in visibleFiles"
                :key="file.name"
                :file="file"
                :selected="filesStore.selected.has(file.name)"
                :current-path="filesStore.currentPath"
                :editing="filesStore.editingName === file.name"
                :is-cut="cutNames.has(file.name)"
                :active="previewName === file.name"
                @select="filesStore.toggleSelection"
                @navigate="filesStore.navigate"
                @download="onDownload"
                @request-rename="filesStore.startRename(file.name)"
                @cancel-rename="filesStore.cancelRename"
                @commit-rename="(name: string) => onCommitRename(file, name)"
                @preview="previewName = file.name"
              />
            </tbody>
          </table>

          <!-- Cards view -->
          <div v-else role="list" class="grid gap-3 p-3 grid-cols-[repeat(auto-fill,minmax(9.5rem,1fr))]">
            <FileCard
              v-for="file in visibleFiles"
              :key="file.name"
              :file="file"
              :selected="filesStore.selected.has(file.name)"
              :current-path="filesStore.currentPath"
              :editing="filesStore.editingName === file.name"
              :is-cut="cutNames.has(file.name)"
              :active="previewName === file.name"
              @select="filesStore.toggleSelection"
              @navigate="filesStore.navigate"
              @download="onDownload"
              @request-rename="filesStore.startRename(file.name)"
              @cancel-rename="filesStore.cancelRename"
              @commit-rename="(name: string) => onCommitRename(file, name)"
              @preview="previewName = file.name"
            />
          </div>
        </div>
      </UContextMenu>

      <FilePreviewPanel
        v-if="previewFile"
        :file="previewFile"
        :dir="filesStore.currentPath"
        class="absolute inset-0 z-20 w-full md:static md:inset-auto md:z-auto md:w-80 lg:w-96 md:shrink-0"
        @close="previewName = null"
      />
    </div>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
