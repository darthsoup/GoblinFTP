<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const uploadStore = useUploadStore()
const { t } = useI18n()

// Blocking: not dismissible, no close button — the only way out is reconnect.
const open = computed(() => authStore.sessionLost)

function reconnect() {
  uploadStore.cancelAll()
  filesStore.$reset()
  authStore.acknowledgeSessionLost()
}
</script>

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
