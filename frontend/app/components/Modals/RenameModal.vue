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
      <div class="p-6 space-y-4 min-w-80">
        <h2 class="text-lg font-semibold">
          {{ t('modal.rename.title') }}
        </h2>
        <UFormField :label="t('modal.rename.label')">
          <UInput
            v-model="newName"
            autofocus
            @keydown.enter="submit"
          />
        </UFormField>
        <UAlert v-if="error" color="error" :description="error" />
        <div class="flex justify-end gap-2">
          <UButton variant="ghost" @click="modalStore.close()">
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
