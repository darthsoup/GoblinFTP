<script setup lang="ts">
const modalStore = useModalStore()
const filesStore = useFilesStore()
const { t } = useI18n()

const name = ref('')
const loading = ref(false)
const error = ref<string | null>(null)

watch(() => modalStore.active, (v) => {
  if (v === 'newFolder') {
    name.value = ''
    error.value = null
  }
})

async function submit() {
  if (!name.value.trim()) {
    error.value = t('modal.newFolder.errorEmpty')
    return
  }
  const dir = filesStore.currentPath.replace(/\/$/, '')
  loading.value = true
  error.value = null
  try {
    await filesStore.mkdir(`${dir}/${name.value.trim()}`)
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
  <UModal :open="modalStore.active === 'newFolder'" @update:open="modalStore.close()">
    <template #content>
      <div class="p-6 space-y-4 min-w-80">
        <h2 class="text-lg font-semibold">
          {{ t('modal.newFolder.title') }}
        </h2>
        <UFormField :label="t('modal.newFolder.label')">
          <UInput
            v-model="name"
            :placeholder="t('modal.newFolder.placeholder')"
            autofocus
            @keydown.enter="submit"
          />
        </UFormField>
        <UAlert v-if="error" color="error" :description="error" />
        <div class="flex justify-end gap-2">
          <UButton variant="ghost" @click="modalStore.close()">
            {{ t('modal.newFolder.cancel') }}
          </UButton>
          <UButton :loading="loading" @click="submit">
            {{ t('modal.newFolder.confirm') }}
          </UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>
