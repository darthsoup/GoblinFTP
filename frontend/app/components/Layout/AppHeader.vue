<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const { t } = useI18n()

async function handleDisconnect() {
  await authStore.disconnect()
  filesStore.$reset()
}
</script>

<template>
  <header class="flex items-center justify-between px-4 h-14 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 shadow-sm">
    <div class="flex items-center gap-2 font-semibold text-lg">
      <UIcon name="i-heroicons-server" class="w-5 h-5 text-primary-500" />
      GoblinFTP
    </div>

    <div class="flex items-center gap-3 text-sm text-gray-500 dark:text-gray-400">
      <UIcon name="i-heroicons-check-circle" class="w-4 h-4 text-green-500" />
      <span>{{ t('header.connected') }}</span>
    </div>

    <UButton
      color="neutral"
      variant="ghost"
      icon="i-heroicons-arrow-right-on-rectangle"
      :label="t('header.disconnect')"
      @click="handleDisconnect"
    />
  </header>
</template>
