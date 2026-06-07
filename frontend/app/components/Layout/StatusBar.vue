<script setup lang="ts">
const authStore = useAuthStore()
const uploadStore = useUploadStore()
const { t } = useI18n()

const activeCount = computed(() =>
  uploadStore.items.filter(i => i.status === 'queued' || i.status === 'uploading').length,
)
</script>

<template>
  <footer class="flex items-center justify-between px-4 h-8 bg-muted border-t border-default font-mono text-xs shrink-0 select-none">
    <div class="flex items-center gap-3 min-w-0">
      <span v-if="authStore.sessionLost" class="flex items-center gap-1.5 shrink-0">
        <span class="size-2 rounded-full bg-error" />
        <span class="text-error">{{ t('header.disconnected') }}</span>
      </span>
      <span v-else class="flex items-center gap-1.5 shrink-0">
        <span class="size-2 rounded-full bg-primary animate-pulse shadow-[0_0_8px_rgba(103,223,112,0.6)]" />
        <span class="text-primary">{{ t('header.connected') }}</span>
      </span>
      <template v-if="authStore.serverHost">
        <span class="text-dimmed">|</span>
        <span class="gap-1.5 sm:inline-flex items-center">
          <UIcon name="i-lucide-plug" class="size-3.5 text-primary shrink-0" />
          <span class="text-muted truncate">{{ authStore.serverHost }}</span>
        </span>
      </template>
    </div>

    <div class="flex items-center gap-3 shrink-0">
      <span class="text-muted">{{ t('status.queue', { n: activeCount }) }}</span>
    </div>
  </footer>
</template>
