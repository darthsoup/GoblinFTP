<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const modalStore = useModalStore()
const { t } = useI18n()

async function handleDisconnect() {
  await authStore.disconnect()
  filesStore.$reset()
}
</script>

<template>
  <UHeader
    title="GoblinFTP"
    :toggle="false"
    :ui="{
      root: 'bg-muted/75 shrink-0 z-30',
      container: 'max-w-full px-4 sm:px-4 lg:px-4 gap-2',
    }"
  >
    <template #left>
      <div class="flex items-center gap-2 select-none">
        <UIcon name="i-lucide-server" class="size-5 text-primary" />
        <span class="text-xl font-bold tracking-tight text-primary">GoblinFTP</span>
      </div>
    </template>

    <template #right>
      <!-- Connected server, surfaced in the bar instead of only the footer. -->
      <span
        v-if="authStore.serverHost"
        class="hidden sm:inline-flex items-center gap-1.5 font-mono text-xs text-muted max-w-48 mr-1"
      >
        <UIcon name="i-lucide-plug" class="size-3.5 text-primary shrink-0" />
        <span class="truncate">{{ authStore.serverHost }}</span>
      </span>

      <UColorModeButton />

      <UTooltip :text="t('header.settings')">
        <UButton
          color="neutral"
          variant="ghost"
          icon="i-lucide-settings"
          :aria-label="t('header.settings')"
          @click="modalStore.open('settings')"
        />
      </UTooltip>

      <USeparator orientation="vertical" class="h-5 mx-1" />

      <UButton
        color="error"
        variant="ghost"
        icon="i-lucide-log-out"
        :label="t('header.disconnect')"
        @click="handleDisconnect"
      />
    </template>
  </UHeader>
</template>
