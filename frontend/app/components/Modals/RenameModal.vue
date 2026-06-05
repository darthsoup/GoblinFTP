<script setup lang="ts">
const modalStore = useModalStore()
const filesStore = useFilesStore()
const { t } = useI18n()

const file = computed(() => modalStore.context.file)
const newName = ref('')
const loading = ref(false)
const error = ref<string | null>(null)

watch(() => modalStore.active, (v) => {
  if (v === 'rename' && file.value) {
    newName.value = file.value.name
    error.value = null
  }
})

const fullPath = computed(() => {
  if (!file.value)
    return ''
  const dir = filesStore.currentPath.replace(/\/$/, '')
  return `${dir}/${file.value.name}`
})

async function submit() {
  if (!file.value)
    return
  const trimmed = newName.value.trim()
  if (!trimmed) {
    error.value = t('modal.rename.errorEmpty')
    return
  }
  if (trimmed === file.value.name) {
    modalStore.close()
    return
  }
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const from = `${dir}/${file.value.name}`
  const to = `${dir}/${trimmed}`
  loading.value = true
  error.value = null
  try {
    await filesStore.rename(from, to)
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
  <UModal :open="modalStore.active === 'rename'" @update:open="modalStore.close()">
    <template #content>
      <div class="flex flex-col min-w-96">
        <!-- Header -->
        <div class="flex items-center justify-between px-4 py-3 border-b border-default bg-elevated/60">
          <h2 class="text-base font-semibold text-highlighted flex items-center gap-2">
            <UIcon name="i-lucide-pencil-line" class="size-5 text-muted" />
            {{ t('modal.rename.title') }}
          </h2>
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            icon="i-lucide-x"
            :aria-label="t('modal.rename.cancel')"
            @click="modalStore.close()"
          />
        </div>

        <!-- Body -->
        <div class="p-5 space-y-4">
          <div>
            <label class="block label-caps text-muted mb-1">{{ t('modal.rename.label') }}</label>
            <UInput
              v-model="newName"
              class="w-full font-mono"
              autofocus
              @keydown.enter="submit"
            />
            <p v-if="file" class="font-mono text-xs text-dimmed truncate mt-1.5" :title="fullPath">
              {{ fullPath }}
            </p>
          </div>
          <UAlert v-if="error" color="error" variant="soft" :description="error" />
        </div>

        <!-- Footer -->
        <div class="flex justify-end gap-2 px-4 py-3 border-t border-default bg-elevated/60">
          <UButton color="neutral" variant="subtle" @click="modalStore.close()">
            {{ t('modal.rename.cancel') }}
          </UButton>
          <UButton :loading="loading" @click="submit">
            {{ t('modal.rename.confirm') }}
          </UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>
