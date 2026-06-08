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
    <div class="flex items-center gap-3 min-w-0" aria-live="polite">
      <span v-if="authStore.sessionLost" class="flex items-center gap-1.5 shrink-0">
        <span class="size-2 rounded-full bg-error" />
        <span class="text-error">{{ t('header.disconnected') }}</span>
      </span>
      <span v-else class="flex items-center gap-1.5 shrink-0">
        <span class="size-2 rounded-full bg-primary animate-pulse shadow-[0_0_8px_color-mix(in_oklab,var(--color-goblin-400)_60%,transparent)]" />
        <span class="text-primary">{{ t('header.connected') }}</span>
      </span>
      <template v-if="authStore.serverHost">
        <span class="text-dimmed hidden sm:inline">|</span>
        <span class="hidden sm:inline-flex gap-1.5 items-center min-w-0">
          <UIcon name="i-lucide-plug" class="size-4 text-primary shrink-0" />
          <span class="text-muted truncate">{{ authStore.serverHost }}</span>
        </span>
      </template>
    </div>

    <div class="flex items-center gap-3 shrink-0">
      <UBadge
        :color="activeCount > 0 ? 'primary' : 'neutral'"
        :variant="activeCount > 0 ? 'subtle' : 'soft'"
        size="sm"
        class="font-mono"
      >
        {{ t('status.queue', { n: activeCount }) }}
      </UBadge>
    </div>
  </footer>
</template>
