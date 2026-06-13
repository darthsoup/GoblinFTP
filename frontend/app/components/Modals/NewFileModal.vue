<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'

const modalStore = useModalStore()
const filesStore = useFilesStore()
const notify = useNotify()
const { t } = useI18n()

const open = computed({
  get: () => modalStore.active === 'newFile',
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
    return [{ name: 'name', message: t('modal.newFile.errorEmpty') }]
  return []
}

async function onSubmit(event: FormSubmitEvent<typeof state>) {
  if (loading.value)
    return
  const dir = filesStore.currentPath.replace(/\/$/, '')
  loading.value = true
  apiError.value = null
  try {
    await filesStore.createFile(`${dir}/${event.data.name.trim()}`)
    notify.success(t('toast.fileCreated'))
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
  <UModal v-model:open="open" :title="t('modal.newFile.title')">
    <template #title>
      <UIcon name="i-lucide-file-plus" class="size-5 text-primary" />
      {{ t('modal.newFile.title') }}
    </template>

    <template #body>
      <UForm
        id="new-file-form"
        :state="state"
        :validate="validate"
        class="space-y-4"
        @submit="onSubmit"
      >
        <UFormField name="name" :label="t('modal.newFile.label')">
          <UInput
            v-model="state.name"
            :placeholder="t('modal.newFile.placeholder')"
            class="w-full"
            autofocus
          />
        </UFormField>
        <UAlert v-if="apiError" color="error" variant="soft" :description="apiError" />
      </UForm>
    </template>

    <template #footer="{ close }">
      <UButton color="neutral" variant="subtle" :label="t('modal.newFile.cancel')" @click="close" />
      <UButton type="submit" form="new-file-form" :loading="loading" :label="t('modal.newFile.confirm')" />
    </template>
  </UModal>
</template>
