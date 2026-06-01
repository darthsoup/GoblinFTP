<script setup lang="ts">
import type { FileInfo } from '~/types/api'

const filesStore = useFilesStore()
const modalStore = useModalStore()
const { t } = useI18n()

type SortKey = 'name' | 'size' | 'modified'
const sortKey = ref<SortKey>('name')
const sortAsc = ref(true)

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
function onContextMenuDelete(file: FileInfo) {
  modalStore.open('delete', { file })
}
function onContextMenuChmod(file: FileInfo) {
  modalStore.open('chmod', { file })
}
function onContextMenuProperties(file: FileInfo) {
  modalStore.open('properties', { file })
}
</script>

<template>
  <div class="flex flex-col flex-1 overflow-hidden">
    <FileToolbar />

    <div class="flex-1 overflow-auto">
      <table class="w-full text-sm text-left">
        <thead class="bg-gray-100 dark:bg-gray-800 sticky top-0">
          <tr>
            <th class="w-8 px-3 py-2">
              <input type="checkbox" class="rounded" @change="filesStore.clearSelection()">
            </th>
            <th class="px-3 py-2 cursor-pointer hover:text-primary-500 font-medium" @click="toggleSort('name')">
              {{ t('files.name') }}
              <UIcon v-if="sortKey === 'name'" :name="sortAsc ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-3 h-3 inline ml-1" />
            </th>
            <th class="px-3 py-2 text-right cursor-pointer hover:text-primary-500 font-medium w-24" @click="toggleSort('size')">
              {{ t('files.size') }}
              <UIcon v-if="sortKey === 'size'" :name="sortAsc ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-3 h-3 inline ml-1" />
            </th>
            <th class="px-3 py-2 cursor-pointer hover:text-primary-500 font-medium w-40" @click="toggleSort('modified')">
              {{ t('files.modified') }}
              <UIcon v-if="sortKey === 'modified'" :name="sortAsc ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-3 h-3 inline ml-1" />
            </th>
            <th class="px-3 py-2 font-medium w-28 hidden sm:table-cell">
              {{ t('files.permissions') }}
            </th>
            <th class="w-16" />
          </tr>
        </thead>

        <tbody v-if="filesStore.loading">
          <tr>
            <td colspan="6" class="py-12 text-center text-gray-400">
              <UIcon name="i-heroicons-arrow-path" class="w-6 h-6 animate-spin inline mr-2" />
              {{ t('files.loading') }}
            </td>
          </tr>
        </tbody>

        <tbody v-else-if="filesStore.error">
          <tr>
            <td colspan="6" class="py-8 text-center text-red-500">
              {{ filesStore.error }}
            </td>
          </tr>
        </tbody>

        <tbody v-else-if="sortedFiles.length === 0">
          <tr>
            <td colspan="6" class="py-12 text-center text-gray-400">
              {{ t('files.empty') }}
            </td>
          </tr>
        </tbody>

        <tbody v-else>
          <FileRow
            v-for="file in sortedFiles"
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
      @chmod="onContextMenuChmod"
      @properties="onContextMenuProperties"
    />
  </div>
</template>
