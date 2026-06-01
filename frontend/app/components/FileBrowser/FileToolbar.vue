<script setup lang="ts">
const filesStore = useFilesStore()
const uploadStore = useUploadStore()
const modalStore = useModalStore()
const { t } = useI18n()

const selectedCount = computed(() => filesStore.selected.size)

// Hidden file input ref
const fileInputRef = ref<HTMLInputElement | null>(null)

function openNewFolder() {
  modalStore.open('newFolder')
}

function openNewFile() {
  modalStore.open('newFile')
}

function deleteSelected() {
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const paths = [...filesStore.selected].map(name => `${dir}/${name}`)
  modalStore.open('delete', { files: paths })
}

function triggerUpload() {
  fileInputRef.value?.click()
}

function onFilesSelected(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files || input.files.length === 0)
    return
  uploadStore.addFiles(input.files, filesStore.currentPath)
  // Reset input so the same file can be re-selected later
  input.value = ''
}

async function downloadZip() {
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const paths = [...filesStore.selected].map(name => `${dir}/${name}`)
  await filesStore.downloadZip(paths)
}
</script>

<template>
  <div class="flex items-center gap-2 px-3 py-2 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
    <!-- Hidden file input for uploads -->
    <input
      ref="fileInputRef"
      type="file"
      multiple
      class="hidden"
      @change="onFilesSelected"
    >

    <UButton
      size="sm"
      variant="ghost"
      icon="i-heroicons-folder-plus"
      @click="openNewFolder"
    >
      {{ t('toolbar.newFolder') }}
    </UButton>
    <UButton
      size="sm"
      variant="ghost"
      icon="i-heroicons-document-plus"
      @click="openNewFile"
    >
      {{ t('toolbar.newFile') }}
    </UButton>
    <UButton
      size="sm"
      variant="ghost"
      icon="i-heroicons-arrow-up-tray"
      @click="triggerUpload"
    >
      {{ t('toolbar.upload') }}
    </UButton>

    <div class="flex-1" />

    <template v-if="selectedCount > 0">
      <span class="text-sm text-gray-500">
        {{ t('toolbar.selected', { n: selectedCount }) }}
      </span>
      <UButton
        v-if="selectedCount >= 2"
        size="sm"
        variant="ghost"
        icon="i-heroicons-archive-box-arrow-down"
        @click="downloadZip"
      >
        {{ t('toolbar.downloadZip') }}
      </UButton>
      <UButton
        size="sm"
        color="error"
        variant="ghost"
        icon="i-heroicons-trash"
        @click="deleteSelected"
      >
        {{ t('toolbar.delete') }}
      </UButton>
    </template>
  </div>
</template>
