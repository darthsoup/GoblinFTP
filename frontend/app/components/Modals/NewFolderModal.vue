<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'

const modalStore = useModalStore()
const filesStore = useFilesStore()
const notify = useNotify()
const { t } = useI18n()

const open = computed({
  get: () => modalStore.active === 'newFolder',
  set: (v: boolean) => {
    if (!v)
      modalStore.close()
  },
})

const state = reactive({ name: '' })
const loading = ref(false)
const apiError = ref<string | null>(null)

watch(open, (v) => {
  if (v) {
    state.name = ''
    apiError.value = null
  }
})

function validate(s: Partial<typeof state>): FormError[] {
  if (!s.name?.trim())
    return [{ name: 'name', message: t('modal.newFolder.errorEmpty') }]
  return []
}

async function onSubmit(event: FormSubmitEvent<typeof state>) {
  if (loading.value)
    return
  const dir = filesStore.currentPath.replace(/\/$/, '')
  loading.value = true
  apiError.value = null
  try {
    await filesStore.mkdir(`${dir}/${event.data.name.trim()}`)
    notify.success(t('toast.folderCreated'))
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
  <UModal v-model:open="open" :title="t('modal.newFolder.title')">
    <template #title>
      <UIcon name="i-lucide-folder-plus" class="size-5 text-primary" />
      {{ t('modal.newFolder.title') }}
    </template>

    <template #body>
      <UForm
        id="new-folder-form"
        :state="state"
        :validate="validate"
        class="space-y-4"
        @submit="onSubmit"
      >
        <UFormField name="name" :label="t('modal.newFolder.label')">
          <UInput
            v-model="state.name"
            :placeholder="t('modal.newFolder.placeholder')"
            class="w-full font-mono"
            autofocus
          />
        </UFormField>
        <UAlert v-if="apiError" color="error" variant="soft" :description="apiError" />
      </UForm>
    </template>

    <template #footer="{ close }">
      <UButton color="neutral" variant="subtle" :label="t('modal.newFolder.cancel')" @click="close" />
      <UButton type="submit" form="new-folder-form" :loading="loading" :label="t('modal.newFolder.confirm')" />
    </template>
  </UModal>
</template>
