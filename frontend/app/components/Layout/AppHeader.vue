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
  <header class="flex items-center justify-between px-4 h-14 bg-muted border-b border-default shrink-0 z-20">
    <div class="flex items-center gap-2 select-none">
      <UIcon name="i-lucide-server" class="size-5 text-primary" />
      <span class="text-xl font-bold tracking-tight text-primary">GoblinFTP</span>
    </div>

    <UButton
      color="neutral"
      variant="ghost"
      icon="i-lucide-log-out"
      :label="t('header.disconnect')"
      @click="handleDisconnect"
    />
  </header>
</template>
