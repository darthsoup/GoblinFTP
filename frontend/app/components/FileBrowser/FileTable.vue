<script setup lang="ts">
import type { FileInfo } from '~/types/api'

const filesStore = useFilesStore()
const modalStore = useModalStore()
const editorStore = useEditorStore()
const { t } = useI18n()

type SortKey = 'name' | 'size' | 'modified'
const sortKey = ref<SortKey>('name')
const sortAsc = ref(true)

const filter = ref('')
watch(() => filesStore.currentPath, () => {
  filter.value = ''
})

const menu = reactive({ visible: false, file: null as FileInfo | null, x: 0, y: 0 })

function toggleSort(key: SortKey) {
  if (sortKey.value === key) {
    sortAsc.value = !sortAsc.value
  }
  else {
    sortKey.value = key
    sortAsc.value = true
  }
}

const sortedFiles = computed(() => {
  const arr = [...filesStore.files]
  // Directories always first
  arr.sort((a, b) => {
    if (a.isDir !== b.isDir)
      return a.isDir ? -1 : 1
    const av = a[sortKey.value]
    const bv = b[sortKey.value]
    const cmp = typeof av === 'string' ? av.localeCompare(bv as string) : (av as number) - (bv as number)
    return sortAsc.value ? cmp : -cmp
  })
  return arr
})

const visibleFiles = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q)
    return sortedFiles.value
  return sortedFiles.value.filter(f => f.name.toLowerCase().includes(q))
})

const allSelected = computed(() =>
  visibleFiles.value.length > 0 && visibleFiles.value.every(f => filesStore.selected.has(f.name)),
)

function toggleSelectAll() {
  if (allSelected.value)
    filesStore.clearSelection()
  else
    filesStore.setSelection(visibleFiles.value.map(f => f.name))
}

function openContextMenu(file: FileInfo, x: number, y: number) {
  menu.file = file
  menu.x = x
  menu.y = y
  menu.visible = true
}

function onContextMenuDownload(file: FileInfo) {
  const dir = filesStore.currentPath.replace(/\/$/, '')
  filesStore.downloadFile(`${dir}/${file.name}`)
}
function onContextMenuRename(file: FileInfo) {
  modalStore.open('rename', { file })
}
function onContextMenuEdit(file: FileInfo) {
  const dir = filesStore.currentPath.replace(/\/$/, '')
  editorStore.openFile(`${dir}/${file.name}`)
}
function onContextMenuDelete(file: FileInfo) {
  modalStore.open('delete', { file })
}
function onContextMenuProperties(file: FileInfo) {
  modalStore.open('properties', { file })
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

    <div class="flex-1 overflow-auto">
      <table class="w-full text-left border-collapse">
        <thead class="sticky top-0 z-[5] bg-muted/95 backdrop-blur label-caps text-muted">
          <tr class="border-b border-default shadow-sm">
            <th class="w-10 px-3 py-2.5 text-center">
              <input
                type="checkbox"
                class="rounded align-middle"
                :checked="allSelected"
                @change="toggleSelectAll"
              >
            </th>
            <th class="w-12 px-2 py-2.5 text-center font-bold">
              {{ t('files.type') }}
            </th>
            <th class="px-3 py-2.5 cursor-pointer hover:text-primary font-bold transition-colors" @click="toggleSort('name')">
              {{ t('files.name') }}
              <UIcon v-if="sortKey === 'name'" :name="sortAsc ? 'i-lucide-chevron-up' : 'i-lucide-chevron-down'" class="size-3 inline ml-1 align-middle" />
            </th>
            <th class="w-24 px-3 py-2.5 text-right cursor-pointer hover:text-primary font-bold transition-colors" @click="toggleSort('size')">
              {{ t('files.size') }}
              <UIcon v-if="sortKey === 'size'" :name="sortAsc ? 'i-lucide-chevron-up' : 'i-lucide-chevron-down'" class="size-3 inline ml-1 align-middle" />
            </th>
            <th class="w-40 px-3 py-2.5 text-right cursor-pointer hover:text-primary font-bold transition-colors" @click="toggleSort('modified')">
              {{ t('files.modified') }}
              <UIcon v-if="sortKey === 'modified'" :name="sortAsc ? 'i-lucide-chevron-up' : 'i-lucide-chevron-down'" class="size-3 inline ml-1 align-middle" />
            </th>
            <th class="w-28 px-3 py-2.5 text-center font-bold hidden sm:table-cell">
              {{ t('files.permissions') }}
            </th>
            <th class="w-14" />
          </tr>
        </thead>

        <tbody v-if="filesStore.loading">
          <tr>
            <td colspan="7" class="py-12 text-center text-muted font-mono text-sm">
              <UIcon name="i-lucide-loader-circle" class="size-5 animate-spin inline mr-2 align-middle text-primary" />
              {{ t('files.loading') }}
            </td>
          </tr>
        </tbody>

        <tbody v-else-if="filesStore.error">
          <tr>
            <td colspan="7" class="py-8 text-center text-error font-mono text-sm">
              <UIcon name="i-lucide-triangle-alert" class="size-5 inline mr-2 align-middle" />
              {{ filesStore.error }}
            </td>
          </tr>
        </tbody>

        <tbody v-else-if="visibleFiles.length === 0">
          <tr>
            <td colspan="7" class="py-16 text-center text-dimmed font-mono text-sm">
              <UIcon name="i-lucide-folder-open" class="size-8 block mx-auto mb-2" />
              {{ filter ? t('files.noMatches') : t('files.empty') }}
            </td>
          </tr>
        </tbody>

        <tbody v-else class="font-mono">
          <FileRow
            v-for="file in visibleFiles"
            :key="file.name"
            :file="file"
            :selected="filesStore.selected.has(file.name)"
            :current-path="filesStore.currentPath"
            @select="filesStore.toggleSelection"
            @navigate="filesStore.navigate"
            @download="filesStore.downloadFile"
            @contextmenu="openContextMenu"
          />
        </tbody>
      </table>
    </div>

    <ContextMenu
      :file="menu.file"
      :x="menu.x"
      :y="menu.y"
      :visible="menu.visible"
      @close="menu.visible = false"
      @download="onContextMenuDownload"
      @rename="onContextMenuRename"
      @delete="onContextMenuDelete"
      @properties="onContextMenuProperties"
      @edit="onContextMenuEdit"
    />
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
