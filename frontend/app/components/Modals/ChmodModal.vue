<script setup lang="ts">
const modalStore = useModalStore()
const filesStore = useFilesStore()
const { t } = useI18n()

const file = computed(() => modalStore.context.file)
const octal = ref('')
const loading = ref(false)
const error = ref<string | null>(null)

// Parse "drwxr-xr-x" → "755" for pre-fill
function modeStringToOctal(mode: string): string {
  // mode string is like "-rwxr-xr-x" or "drwxr-xr-x"
  // take chars 1-9 (skip type char)
  const perms = mode.slice(1, 10)
  if (perms.length < 9)
    return ''
  function triplet(r: string, w: string, x: string): number {
    return (r !== '-' ? 4 : 0) + (w !== '-' ? 2 : 0) + (x !== '-' ? 1 : 0)
  }
  const u = triplet(perms[0], perms[1], perms[2])
  const g = triplet(perms[3], perms[4], perms[5])
  const o = triplet(perms[6], perms[7], perms[8])
  return `${u}${g}${o}`
}

watch(() => modalStore.active, (v) => {
  if (v === 'chmod' && file.value) {
    octal.value = modeStringToOctal(file.value.mode)
    error.value = null
  }
})

function isValidOctal(s: string): boolean {
  return /^[0-7]{3,4}$/.test(s)
}

async function submit() {
  if (!file.value || !isValidOctal(octal.value)) {
    error.value = t('modal.chmod.note')
    return
  }
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const path = `${dir}/${file.value.name}`
  const mode = Number.parseInt(octal.value, 8)
  loading.value = true
  error.value = null
  try {
    await filesStore.chmod(path, mode)
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
  <UModal :open="modalStore.active === 'chmod'" @update:open="modalStore.close()">
    <template #content>
      <div class="p-6 space-y-4 min-w-80">
        <h2 class="text-lg font-semibold">
          {{ t('modal.chmod.title') }}
          <span v-if="file" class="text-sm font-normal text-gray-500 ml-2">{{ file.name }}</span>
        </h2>
        <UFormField :label="t('modal.chmod.label')" :hint="t('modal.chmod.note')">
          <UInput
            v-model="octal"
            :placeholder="t('modal.chmod.placeholder')"
            maxlength="4"
            autofocus
            @keydown.enter="submit"
          />
        </UFormField>
        <UAlert v-if="error" color="error" :description="error" />
        <div class="flex justify-end gap-2">
          <UButton variant="ghost" @click="modalStore.close()">
            {{ t('modal.chmod.cancel') }}
          </UButton>
          <UButton :loading="loading" @click="submit">
            {{ t('modal.chmod.confirm') }}
          </UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>
