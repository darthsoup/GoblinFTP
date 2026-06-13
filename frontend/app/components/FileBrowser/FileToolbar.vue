<script setup lang="ts">
import { ApiError } from '~/types/api'

const filter = defineModel<string>('filter', { default: '' })

const filesStore = useFilesStore()
const uploadStore = useUploadStore()
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const notify = useNotify()
const { t } = useI18n()

const selectedCount = computed(() => filesStore.selected.size)

function toggleView() {
  settingsStore.fileViewMode = settingsStore.fileViewMode === 'table' ? 'cards' : 'table'
}

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
  try {
    await filesStore.downloadZip(paths)
  }
  catch (e) {
    notify.error(e instanceof ApiError ? e.message : t('toast.downloadFailed'))
  }
}

function copySelected() {
  filesStore.copyToClipboard([...filesStore.selected])
}

function cutSelected() {
  filesStore.cutToClipboard([...filesStore.selected])
}

const paste = usePaste()
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

    <USeparator orientation="vertical" class="h-5 mx-1" />

    <UTooltip :text="t('toolbar.refresh')">
      <UButton
        size="sm"
        color="neutral"
        variant="subtle"
        icon="i-lucide-refresh-cw"
        :aria-label="t('toolbar.refresh')"
        :loading="filesStore.loading"
        @click="filesStore.list()"
      />
    </UTooltip>
    <UTooltip :text="t('toolbar.upload')">
      <UButton
        size="sm"
        color="neutral"
        variant="subtle"
        icon="i-lucide-upload"
        :aria-label="t('toolbar.upload')"
        @click="triggerUpload"
      />
    </UTooltip>
    <UTooltip :text="t('shortcuts.title')">
      <UButton
        size="sm"
        color="neutral"
        variant="ghost"
        icon="i-lucide-keyboard"
        :aria-label="t('shortcuts.title')"
        class="hidden sm:inline-flex"
        @click="modalStore.open('shortcuts')"
      />
    </UTooltip>
    <UTooltip :text="t('toolbar.viewToggle')">
      <UButton
        size="sm"
        color="neutral"
        variant="ghost"
        :icon="settingsStore.fileViewMode === 'table' ? 'i-lucide-layout-grid' : 'i-lucide-table'"
        :aria-label="t('toolbar.viewToggle')"
        @click="toggleView"
      />
    </UTooltip>

    <template v-if="filesStore.clipboard">
      <USeparator orientation="vertical" class="h-5 mx-1" />
      <UButton
        size="sm"
        color="primary"
        variant="subtle"
        icon="i-lucide-clipboard-paste"
        @click="paste"
      >
        {{ t('toolbar.paste', { n: filesStore.clipboard.names.length }) }}
      </UButton>
    </template>

    <div class="flex-1" />

    <template v-if="selectedCount > 0">
      <span class="text-xs text-muted">
        {{ t('toolbar.selected', { n: selectedCount }) }}
      </span>
      <UButton
        size="sm"
        color="neutral"
        variant="subtle"
        icon="i-lucide-copy"
        @click="copySelected"
      >
        {{ t('toolbar.copy') }}
      </UButton>
      <UButton
        size="sm"
        color="neutral"
        variant="subtle"
        icon="i-lucide-scissors"
        @click="cutSelected"
      >
        {{ t('toolbar.cut') }}
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
    </template>

    <UInput
      v-model="filter"
      size="sm"
      icon="i-lucide-search"
      :placeholder="t('toolbar.filter')"
      class="w-full sm:w-44 md:w-56"
    />
  </div>
</template>
