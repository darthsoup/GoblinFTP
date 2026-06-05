<script setup lang="ts">
const modalStore = useModalStore()
const filesStore = useFilesStore()
const { t } = useI18n()

const name = ref('')
const loading = ref(false)
const error = ref<string | null>(null)

watch(() => modalStore.active, (v) => {
  if (v === 'newFile') {
    name.value = ''
    error.value = null
  }
})

async function submit() {
  if (!name.value.trim()) {
    error.value = t('modal.newFile.errorEmpty')
    return
  }
  const dir = filesStore.currentPath.replace(/\/$/, '')
  loading.value = true
  error.value = null
  try {
    await filesStore.createFile(`${dir}/${name.value.trim()}`)
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
  <UModal :open="modalStore.active === 'newFile'" @update:open="modalStore.close()">
    <template #content>
      <div class="flex flex-col min-w-96">
        <!-- Header -->
        <div class="flex items-center justify-between px-4 py-3 border-b border-default bg-elevated/60">
          <h2 class="text-base font-semibold text-highlighted flex items-center gap-2">
            <UIcon name="i-lucide-file-plus" class="size-5 text-primary" />
            {{ t('modal.newFile.title') }}
          </h2>
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            icon="i-lucide-x"
            :aria-label="t('modal.newFile.cancel')"
            @click="modalStore.close()"
          />
        </div>

        <!-- Body -->
        <div class="p-5 space-y-4">
          <div>
            <label class="block label-caps text-muted mb-1">{{ t('modal.newFile.label') }}</label>
            <UInput
              v-model="name"
              :placeholder="t('modal.newFile.placeholder')"
              class="w-full font-mono"
              autofocus
              @keydown.enter="submit"
            />
          </div>
          <UAlert v-if="error" color="error" variant="soft" :description="error" />
        </div>

        <!-- Footer -->
        <div class="flex justify-end gap-2 px-4 py-3 border-t border-default bg-elevated/60">
          <UButton color="neutral" variant="subtle" @click="modalStore.close()">
            {{ t('modal.newFile.cancel') }}
          </UButton>
          <UButton :loading="loading" @click="submit">
            {{ t('modal.newFile.confirm') }}
          </UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>
