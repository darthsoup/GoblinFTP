<script setup lang="ts">
const modalStore = useModalStore()
const filesStore = useFilesStore()
const { t } = useI18n()

const loading = ref(false)
const error = ref<string | null>(null)

// context.files = explicit path list (bulk); context.file = single file from context menu
const paths = computed<string[]>(() => {
  if (modalStore.context.files?.length)
    return modalStore.context.files
  if (modalStore.context.file) {
    const dir = filesStore.currentPath.replace(/\/$/, '')
    return [`${dir}/${modalStore.context.file.name}`]
  }
  return []
})

const message = computed(() => {
  if (paths.value.length === 1 && modalStore.context.file)
    return t('modal.delete.message', { name: modalStore.context.file.name })
  return t('modal.delete.messageMulti', { n: paths.value.length })
})

async function confirm() {
  if (!paths.value.length)
    return
  loading.value = true
  error.value = null
  try {
    await filesStore.deleteFiles(paths.value)
    modalStore.close()
  }
  catch (e) {
    error.value = e instanceof Error ? e.message : t('error.operationFailed')
  }
  finally {
    loading.value = false
  }
}
</script>

<template>
  <UModal :open="modalStore.active === 'delete'" @update:open="modalStore.close()">
    <template #content>
      <div class="p-6 space-y-4 min-w-80">
        <h2 class="text-lg font-semibold">
          {{ t('modal.delete.title') }}
        </h2>
        <p class="text-gray-600 dark:text-gray-400">
          {{ message }}
        </p>
        <UAlert v-if="error" color="error" :description="error" />
        <div class="flex justify-end gap-2">
          <UButton variant="ghost" @click="modalStore.close()">
            {{ t('modal.delete.cancel') }}
          </UButton>
          <UButton color="error" :loading="loading" @click="confirm">
            {{ t('modal.delete.confirm') }}
          </UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>
