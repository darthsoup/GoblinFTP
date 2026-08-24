<template>
  <UModal :open="open" :dismissible="false" :close="false" :title="t('session.lostTitle')">
    <template #title>
      <UIcon name="i-lucide-unplug" class="size-5 text-error" />
      {{ t('session.lostTitle') }}
    </template>

    <template #body>
      <p class="text-muted">
        {{ t('session.lostMessage') }}
      </p>
    </template>

    <template #footer>
      <UButton icon="i-lucide-plug" :label="t('session.reconnect')" @click="reconnect" />
    </template>
  </UModal>
</template>

<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const uploadStore = useUploadStore()
const { t } = useI18n()

// Blocking: not dismissible, no close button, so the only way out is reconnect.
const open = computed(() => authStore.sessionLost)

function reconnect() {
  uploadStore.cancelAll()
  filesStore.$reset()
  // The editor is deliberately NOT reset: this modal is not dismissible, so
  // wiping it destroyed every unsaved buffer with no prompt (auto-save is off
  // by default). The tabs survive the reconnect and stay saveable.
  authStore.acknowledgeSessionLost()
}
</script>
