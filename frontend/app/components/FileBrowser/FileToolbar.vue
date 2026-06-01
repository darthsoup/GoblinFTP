<script setup lang="ts">
const filesStore = useFilesStore()
const modalStore = useModalStore()
const { t } = useI18n()

const selectedCount = computed(() => filesStore.selected.size)

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
</script>

<template>
  <div class="flex items-center gap-2 px-3 py-2 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
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

    <div class="flex-1" />

    <template v-if="selectedCount > 0">
      <span class="text-sm text-gray-500">
        {{ t('toolbar.selected', { n: selectedCount }) }}
      </span>
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
