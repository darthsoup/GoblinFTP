<script setup lang="ts">
import { ApiError } from '~/types/api'

const modalStore = useModalStore()
const filesStore = useFilesStore()
const notify = useNotify()
const { localizeError, formatFailures } = useErrorMessage()
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
    const res = await filesStore.deleteFiles(paths.value)
    if (res.failed.length === 0) {
      notify.success(t('toast.deleted', { n: res.deleted.length }))
    }
    else {
      notify.error(
        t('toast.deleteFailed', { n: res.failed.length }),
        formatFailures(res.failed.map(f => ({ label: basename(f.path), reason: localizeError(f.code, f.message) }))),
      )
    }
    modalStore.close()
  }
  catch (e) {
    // Whole-request failure (e.g. bad request); session/connection loss routes to
    // the reconnect dialog via useApi.
    apiError.value = e instanceof ApiError ? localizeError(e.code, e.message) : t('error.operationFailed')
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
