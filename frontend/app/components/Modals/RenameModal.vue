<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'

const modalStore = useModalStore()
const filesStore = useFilesStore()
const { t } = useI18n()

const file = computed(() => modalStore.context.file)
const open = computed({
  get: () => modalStore.active === 'rename',
  set: (v: boolean) => {
    if (!v)
      modalStore.close()
  },
})

const state = reactive({ name: '' })
const loading = ref(false)
const apiError = ref<string | null>(null)

watch(open, (v) => {
  if (v && file.value) {
    state.name = file.value.name
    apiError.value = null
  }
})

const fullPath = computed(() => {
  if (!file.value)
    return ''
  const dir = filesStore.currentPath.replace(/\/$/, '')
  return `${dir}/${file.value.name}`
})

function validate(s: Partial<typeof state>): FormError[] {
  if (!s.name?.trim())
    return [{ name: 'name', message: t('modal.rename.errorEmpty') }]
  return []
}

async function onSubmit(event: FormSubmitEvent<typeof state>) {
  if (!file.value || loading.value)
    return
  const trimmed = event.data.name.trim()
  if (trimmed === file.value.name) {
    modalStore.close()
    return
  }
  const dir = filesStore.currentPath.replace(/\/$/, '')
  loading.value = true
  apiError.value = null
  try {
    await filesStore.rename(`${dir}/${file.value.name}`, `${dir}/${trimmed}`)
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
  <UModal v-model:open="open" :title="t('modal.rename.title')">
    <template #title>
      <UIcon name="i-lucide-pencil-line" class="size-5 text-muted" />
      {{ t('modal.rename.title') }}
    </template>

    <template #body>
      <UForm
        id="rename-form"
        :state="state"
        :validate="validate"
        class="space-y-4"
        @submit="onSubmit"
      >
        <UFormField name="name" :label="t('modal.rename.label')">
          <UInput v-model="state.name" class="w-full font-mono" autofocus />
          <template #help>
            <span class="block font-mono text-xs text-dimmed truncate" :title="fullPath">{{ fullPath }}</span>
          </template>
        </UFormField>
        <UAlert v-if="apiError" color="error" variant="soft" :description="apiError" />
      </UForm>
    </template>

    <template #footer="{ close }">
      <UButton color="neutral" variant="subtle" :label="t('modal.rename.cancel')" @click="close" />
      <UButton type="submit" form="rename-form" :loading="loading" :label="t('modal.rename.confirm')" />
    </template>
  </UModal>
</template>
