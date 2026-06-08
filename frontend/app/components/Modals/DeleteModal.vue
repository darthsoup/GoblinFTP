<script setup lang="ts">
const modalStore = useModalStore()
const filesStore = useFilesStore()
const notify = useNotify()
const { t } = useI18n()

const open = computed({
  get: () => modalStore.active === 'delete',
  set: (v: boolean) => {
    if (!v)
      modalStore.close()
  },
})

const loading = ref(false)
const apiError = ref<string | null>(null)

watch(open, (v) => {
  if (v)
    apiError.value = null
})

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
  if (!paths.value.length || loading.value)
    return
  loading.value = true
  apiError.value = null
  try {
    const n = paths.value.length
    await filesStore.deleteFiles(paths.value)
    notify.success(t('toast.deleted', { n }))
    modalStore.close()
  }
  catch (e) {
    apiError.value = e instanceof Error ? e.message : t('error.operationFailed')
  }
  finally {
    loading.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" :title="t('modal.delete.title')">
    <template #title>
      <UIcon name="i-lucide-triangle-alert" class="size-5 text-error" />
      {{ t('modal.delete.title') }}
    </template>

    <template #body>
      <div class="space-y-4">
        <p class="text-muted">
          {{ message }}
        </p>
        <UAlert v-if="apiError" color="error" variant="soft" :description="apiError" />
      </div>
    </template>

    <template #footer="{ close }">
      <UButton color="neutral" variant="subtle" :label="t('modal.delete.cancel')" @click="close" />
      <UButton color="error" :loading="loading" :label="t('modal.delete.confirm')" @click="confirm" />
    </template>
  </UModal>
</template>
