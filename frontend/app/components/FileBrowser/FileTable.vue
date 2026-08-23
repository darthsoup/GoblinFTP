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
          <!-- Loading, error and empty states, shared by both views -->
          <div v-if="filesStore.loading" class="py-12 text-center text-muted text-sm">
            <UIcon name="i-lucide-loader-circle" class="size-5 animate-spin inline-block mr-2 align-middle text-primary" />
            {{ t('files.loading') }}
          </div>

          <div v-else-if="filesStore.error" class="py-8 flex flex-col items-center gap-3">
            <p class="text-error text-sm text-center px-4">
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

          <div v-else-if="visibleFiles.length === 0" class="flex flex-col items-center justify-center gap-4 py-16 text-center">
            <div class="relative flex items-center justify-center">
              <div class="absolute size-20 rounded-full bg-primary/10 blur-xl" aria-hidden="true" />
              <div class="relative flex size-16 items-center justify-center rounded-2xl border border-default bg-elevated/60 text-primary">
                <UIcon :name="filter ? 'i-lucide-search-x' : 'i-lucide-folder-open'" class="size-8" />
              </div>
            </div>
            <div class="space-y-1">
              <p class="text-sm font-semibold text-highlighted">
                {{ filter ? t('files.noMatches') : t('files.empty') }}
              </p>
              <p v-if="!filter" class="text-xs text-dimmed">
                {{ t('files.dropToUpload') }}
              </p>
            </div>
            <div class="flex items-center gap-2">
              <UButton v-for="(action, idx) in emptyActions" :key="idx" v-bind="action" />
            </div>
          </div>

          <table v-else-if="viewMode === 'table'" class="w-full text-left border-collapse" :aria-label="t('files.listOf', { path: filesStore.currentPath })">
            <thead class="sticky top-0 z-[5] bg-elevated/95 backdrop-blur label-caps text-muted">
              <tr class="border-b border-default shadow-sm">
                <th class="w-10 px-4 py-2">
                  <UCheckbox
                    :model-value="headerChecked"
                    size="md"
                    class="justify-center"
                    :aria-label="allSelected ? t('toolbar.deselectAll') : t('toolbar.selectAll')"
                    @update:model-value="toggleSelectAll"
                  />
                </th>
                <th class="px-3 py-2.5 font-bold" :aria-sort="ariaSort('name')">
                  <button type="button" class="inline-flex items-center cursor-pointer hover:text-primary transition-colors" @click="toggleSort('name')">
                    {{ t('files.name') }}
                    <UIcon :name="sortIcon('name')" class="size-3 inline-block ml-1 align-middle" :class="sortKey === 'name' ? 'text-primary' : 'text-dimmed'" />
                  </button>
                </th>
                <th class="w-24 px-4 py-2 text-right whitespace-nowrap font-bold hidden sm:table-cell" :aria-sort="ariaSort('size')">
                  <button type="button" class="inline-flex items-center cursor-pointer hover:text-primary transition-colors" @click="toggleSort('size')">
                    {{ t('files.size') }}
                    <UIcon :name="sortIcon('size')" class="size-3 inline-block ml-1 align-middle" :class="sortKey === 'size' ? 'text-primary' : 'text-dimmed'" />
                  </button>
                </th>
                <th class="w-40 px-4 py-2 text-right whitespace-nowrap font-bold hidden md:table-cell" :aria-sort="ariaSort('modified')">
                  <button type="button" class="inline-flex items-center cursor-pointer hover:text-primary transition-colors" @click="toggleSort('modified')">
                    {{ t('files.modified') }}
                    <UIcon :name="sortIcon('modified')" class="size-3 inline-block ml-1 align-middle" :class="sortKey === 'modified' ? 'text-primary' : 'text-dimmed'" />
                  </button>
                </th>
                <th v-if="hasPermissions" class="w-28 px-4 py-2 text-center font-bold hidden sm:table-cell">
                  {{ t('files.permissions') }}
                </th>
                <th class="w-14" />
              </tr>
            </thead>

            <tbody>
              <FileRow
                v-for="(file, i) in visibleFiles"
                :key="`${filesStore.currentPath}/${file.name}`"
                :file="file"
                :selected="filesStore.selected.has(file.name)"
                :current-path="filesStore.currentPath"
                :editing="filesStore.editingName === file.name"
                :is-cut="cutNames.has(file.name)"
                :active="previewName === file.name"
                :compact="compact"
                :show-permissions="hasPermissions"
                :index="i"
                :menu-items="buildFileMenu(file)"
                @select="filesStore.toggleSelection"
                @navigate="filesStore.navigate"
                @request-rename="filesStore.startRename(file.name)"
                @cancel-rename="filesStore.cancelRename"
                @commit-rename="(name: string) => onCommitRename(file, name)"
                @preview="previewName = file.name"
                @focus-move="(d: number | 'first' | 'last') => moveRowFocus(i, d)"
              />
            </tbody>
          </table>

          <template v-else>
            <!-- The grid has no column header, so this sticky bar carries the
                 select-all affordance the table gets from its <thead> checkbox. -->
            <div
              class="sticky top-0 z-[5] flex items-center bg-elevated/95 backdrop-blur border-b border-default"
              :class="compact ? 'px-2.5 py-1.5' : 'px-3.5 py-2'"
            >
              <!-- The label stays constant: a count not carried into the accessible
                   name fails WCAG 2.5.3, and Reka freezes a stale one at mount. -->
              <UCheckbox
                :model-value="headerChecked"
                size="md"
                :label="t('toolbar.selectAll')"
                :aria-label="t('toolbar.selectAll')"
                :ui="{ label: 'label-caps text-muted' }"
                @update:model-value="toggleSelectAll"
              />
            </div>

            <div
              role="list"
              class="grid"
              :class="compact
                ? 'gap-2 p-2 grid-cols-[repeat(auto-fill,minmax(7rem,1fr))]'
                : 'gap-3 p-3 grid-cols-[repeat(auto-fill,minmax(9.5rem,1fr))]'"
            >
              <FileCard
                v-for="(file, i) in visibleFiles"
                :key="`${filesStore.currentPath}/${file.name}`"
                :file="file"
                :selected="filesStore.selected.has(file.name)"
                :current-path="filesStore.currentPath"
                :editing="filesStore.editingName === file.name"
                :is-cut="cutNames.has(file.name)"
                :active="previewName === file.name"
                :compact="compact"
                :index="i"
                @select="filesStore.toggleSelection"
                @navigate="filesStore.navigate"
                @download="onDownload"
                @request-rename="filesStore.startRename(file.name)"
                @cancel-rename="filesStore.cancelRename"
                @commit-rename="(name: string) => onCommitRename(file, name)"
                @preview="previewName = file.name"
                @focus-move="(d: number | 'first' | 'last') => moveRowFocus(i, d)"
              />
            </div>
          </template>
        </div>
      </UContextMenu>

      <!-- Inspector overlays the right edge rather than reserving a column, so
           opening it never reflows the list. Non-modal: previews swap in place. -->
      <Transition name="preview">
        <FilePreviewPanel
          v-if="previewFile"
          :file="previewFile"
          :dir="filesStore.currentPath"
          class="absolute inset-y-0 right-0 z-20 w-full sm:w-80 lg:w-96 shadow-xl shadow-black/20"
          @close="previewName = null"
        />
      </Transition>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ButtonProps, DropdownMenuItem } from '@nuxt/ui'
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

// Roving-tabindex focus move. Rows are found by a data-file-name query rather
// than a ref each, since a listing can be thousands of entries.
function moveRowFocus(from: number, delta: number | 'first' | 'last') {
  const rows = Array.from(
    document.querySelectorAll<HTMLElement>('tr.file-row[data-file-name], [data-file-card][data-file-name]'),
  )
  if (rows.length === 0)
    return
  let next: number
  if (delta === 'first')
    next = 0
  else if (delta === 'last')
    next = rows.length - 1
  else
    next = Math.min(rows.length - 1, Math.max(0, from + delta))
  rows[next]?.focus()
}

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

const compact = computed(() => settingsStore.density === 'compact')
// Many FTP servers return no mode, so hide the Permissions column entirely
// rather than filling it with placeholders.
const hasPermissions = computed(() => visibleFiles.value.some(f => !!f.mode))

// Browser-only keyboard shortcuts (select-all matches the visible/filtered set).
useFileBrowserShortcuts(() => visibleFiles.value.map(f => f.name))

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

// Names dimmed as "pending move": only cut items still in their source dir.
const cutNames = computed(() => {
  const cb = filesStore.clipboard
  if (!cb || cb.mode !== 'cut' || cb.sourcePath !== filesStore.currentPath.replace(/\/$/, ''))
    return new Set<string>()
  return new Set(cb.names)
})

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

// Shared by the row/card "⋮" dropdown and the right-click context menu, so both
// expose an identical action set. Grouped arrays render as separated sections.
function buildFileMenu(file: FileInfo): DropdownMenuItem[][] {
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const path = `${dir}/${file.name}`

  // Directories can't be downloaded, so the group is dropped below when empty.
  const primary: DropdownMenuItem[] = []
  if (!file.isDir)
    primary.push({ label: t('context.download'), icon: 'i-lucide-download', onSelect: () => onDownload(path) })

  const middle: DropdownMenuItem[] = [
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

  const clipboard: DropdownMenuItem[] = [
    { label: t('context.copy'), icon: 'i-lucide-copy', onSelect: () => filesStore.copyToClipboard(clipboardNames(file)) },
    { label: t('context.cut'), icon: 'i-lucide-scissors', onSelect: () => filesStore.cutToClipboard(clipboardNames(file)) },
  ]
  if (filesStore.clipboard)
    clipboard.push({ label: t('context.paste'), icon: 'i-lucide-clipboard-paste', onSelect: runPaste })

  const del: DropdownMenuItem[] = [
    { label: t('context.delete'), icon: 'i-lucide-trash-2', color: 'error', onSelect: () => modalStore.open('delete', { file }) },
  ]

  return [primary, middle, clipboard, del].filter(group => group.length > 0)
}

const menuItems = computed<DropdownMenuItem[][]>(() => menuFile.value ? buildFileMenu(menuFile.value) : [])

// Capture-phase: resolve the right-clicked row/card by `[data-file-name]` before
// Reka opens the menu; on empty space the browser's own menu shows instead.
function onAreaContextMenu(e: MouseEvent) {
  const row = (e.target as HTMLElement).closest<HTMLElement>('[data-file-name]')
  const file = row ? visibleFiles.value.find(f => f.name === row.dataset.fileName) : undefined
  if (!file) {
    e.stopPropagation()
    return
  }
  menuFile.value = file
}

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

async function onDrop(e: DragEvent) {
  dragCounter = 0
  isDragOver.value = false
  if (!e.dataTransfer)
    return
  // readDropEntries snapshots the items synchronously (they're only valid during
  // the event), then traverses dropped folders into nested relative paths.
  const { files, emptyDirs } = await readDropEntries(e.dataTransfer)
  const base = filesStore.currentPath.replace(/\/$/, '')
  // Uploads first: they may prompt about conflicts, and cancelling there should
  // not leave freshly created directories behind.
  if (files.length > 0)
    await uploadStore.addEntries(files, filesStore.currentPath)
  if (emptyDirs.length > 0) {
    // Non-empty dirs are created implicitly by the upload; create the empty ones.
    const results = await Promise.allSettled(emptyDirs.map(d => filesStore.ensureDir(`${base}/${d}`)))
    if (results.some(r => r.status === 'rejected'))
      notify.error(t('toast.folderCreateFailed'))
    if (files.length === 0)
      await filesStore.list() // empty-only drop → reveal the new folders now
  }
}
</script>

<style scoped>
/* Inspector slides in from the right edge it's pinned to. */
.preview-enter-active,
.preview-leave-active {
  transition:
    transform 0.2s ease,
    opacity 0.2s ease;
}
.preview-enter-from,
.preview-leave-to {
  transform: translateX(100%);
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .preview-enter-active,
  .preview-leave-active {
    transition: none;
  }
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
