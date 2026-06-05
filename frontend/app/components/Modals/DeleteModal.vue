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
  if (!paths.value.length || loading.value)
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
      <div class="flex flex-col min-w-96">
        <!-- Header -->
        <div class="flex items-center justify-between px-4 py-3 border-b border-default bg-elevated/60">
          <h2 class="text-base font-semibold text-highlighted flex items-center gap-2">
            <UIcon name="i-lucide-triangle-alert" class="size-5 text-error" />
            {{ t('modal.delete.title') }}
          </h2>
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            icon="i-lucide-x"
            :aria-label="t('modal.delete.cancel')"
            @click="modalStore.close()"
          />
        </div>

        <!-- Body -->
        <div class="p-5 space-y-4">
          <p class="text-muted">
            {{ message }}
          </p>
          <UAlert v-if="error" color="error" variant="soft" :description="error" />
        </div>

        <!-- Footer -->
        <div class="flex justify-end gap-2 px-4 py-3 border-t border-default bg-elevated/60">
          <UButton color="neutral" variant="subtle" @click="modalStore.close()">
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
