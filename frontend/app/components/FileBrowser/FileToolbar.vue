<script setup lang="ts">
const filter = defineModel<string>('filter', { default: '' })

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
  <div class="flex flex-wrap items-center gap-2 px-4 py-2 border-b border-default bg-elevated/50 shrink-0">
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
      color="primary"
      icon="i-lucide-folder-plus"
      @click="openNewFolder"
    >
      {{ t('toolbar.newFolder') }}
    </UButton>
    <UButton
      size="sm"
      color="neutral"
      variant="subtle"
      icon="i-lucide-file-plus"
      @click="openNewFile"
    >
      {{ t('toolbar.newFile') }}
    </UButton>

    <div class="h-5 w-px bg-accented mx-1" />

    <UButton
      size="sm"
      color="neutral"
      variant="subtle"
      icon="i-lucide-refresh-cw"
      :aria-label="t('toolbar.refresh')"
      :title="t('toolbar.refresh')"
      :loading="filesStore.loading"
      @click="filesStore.list()"
    />
    <UButton
      size="sm"
      color="neutral"
      variant="subtle"
      icon="i-lucide-upload"
      :aria-label="t('toolbar.upload')"
      :title="t('toolbar.upload')"
      @click="triggerUpload"
    />

    <div class="flex-1" />

    <template v-if="selectedCount > 0">
      <span class="text-xs font-mono text-muted">
        {{ t('toolbar.selected', { n: selectedCount }) }}
      </span>
      <UButton
        v-if="selectedCount >= 2"
        size="sm"
        color="neutral"
        variant="subtle"
        icon="i-lucide-file-archive"
        @click="downloadZip"
      >
        {{ t('toolbar.downloadZip') }}
      </UButton>
      <UButton
        size="sm"
        color="error"
        variant="soft"
        icon="i-lucide-trash-2"
        @click="deleteSelected"
      >
        {{ t('toolbar.delete') }}
      </UButton>
    </template>

    <UInput
      v-model="filter"
      size="sm"
      icon="i-lucide-search"
      :placeholder="t('toolbar.filter')"
      class="w-44 md:w-56 font-mono"
    />
  </div>
</template>
