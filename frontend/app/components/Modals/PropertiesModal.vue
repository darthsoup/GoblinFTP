<script setup lang="ts">
const modalStore = useModalStore()
const { t, locale } = useI18n()

const file = computed(() => modalStore.context.file)

function formatSize(bytes: number): string {
  if (bytes < 1024)
    return `${bytes} B`
  if (bytes < 1048576)
    return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1073741824)
    return `${(bytes / 1048576).toFixed(1)} MB`
  return `${(bytes / 1073741824).toFixed(2)} GB`
}

function formatDate(iso: string): string {
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime()))
      return iso
    return d.toLocaleString(locale.value || 'en-US')
  }
  catch {
    return iso
  }
}

const rows = computed(() => {
  if (!file.value)
    return []
  return [
    { label: t('modal.properties.name'), value: file.value.name },
    { label: t('modal.properties.type'), value: file.value.type },
    { label: t('modal.properties.size'), value: formatSize(file.value.size) },
    { label: t('modal.properties.modified'), value: formatDate(file.value.modified) },
    { label: t('modal.properties.permissions'), value: file.value.mode },
  ]
})
</script>

<template>
  <UModal :open="modalStore.active === 'properties'" @update:open="modalStore.close()">
    <template #content>
      <div class="p-6 space-y-4 min-w-96">
        <h2 class="text-lg font-semibold">
          {{ t('modal.properties.title') }}
        </h2>
        <dl class="divide-y divide-gray-200 dark:divide-gray-700">
          <div
            v-for="row in rows"
            :key="row.label"
            class="flex justify-between py-2 text-sm"
          >
            <dt class="text-gray-500 dark:text-gray-400">
              {{ row.label }}
            </dt>
            <dd class="font-medium">
              {{ row.value }}
            </dd>
          </div>
        </dl>
        <div class="flex justify-end">
          <UButton variant="ghost" @click="modalStore.close()">
            {{ t('modal.properties.close') }}
          </UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>
