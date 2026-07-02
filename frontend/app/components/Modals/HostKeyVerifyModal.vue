<script setup lang="ts">
// Trust-on-first-use prompt for an unknown SFTP host key. Driven directly by
// authStore.pendingHostKey (set during the login/SSO connect flow), so it lives
// on the login screen rather than the file-browser modal store. When the server
// presents a key that differs from the pinned one, the same modal switches to a
// danger-styled re-trust variant showing both fingerprints.
const authStore = useAuthStore()
const { t } = useI18n()

const open = computed({
  get: () => authStore.pendingHostKey !== null,
  set: (v: boolean) => {
    if (!v)
      authStore.cancelHostKey()
  },
})

const prompt = computed(() => authStore.pendingHostKey)
const changed = computed(() => prompt.value?.changed === true)
const loading = ref(false)

async function trust() {
  if (loading.value)
    return
  loading.value = true
  try {
    // The (re)connect surfaces any failure via authStore.error on the form.
    await authStore.confirmHostKey()
  }
  catch {}
  finally {
    loading.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" :title="t(changed ? 'hostKey.changedTitle' : 'hostKey.title')" :dismissible="false">
    <template #title>
      <UIcon
        :name="changed ? 'i-lucide-shield-alert' : 'i-lucide-shield-question'"
        class="size-5"
        :class="changed ? 'text-error' : 'text-warning'"
      />
      {{ t(changed ? 'hostKey.changedTitle' : 'hostKey.title') }}
    </template>

    <template #body>
      <div v-if="prompt" class="space-y-4">
        <p class="text-sm text-muted">
          {{ t(changed ? 'hostKey.changedMessage' : 'hostKey.message') }}
        </p>
        <div class="space-y-2 rounded-md border border-default bg-muted/40 p-3">
          <div v-if="prompt.host" class="flex items-center justify-between gap-3">
            <span class="label-caps text-dimmed">{{ t('hostKey.host') }}</span>
            <span class="font-mono text-xs text-highlighted">{{ prompt.host }}</span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="label-caps text-dimmed">{{ t('hostKey.keyType') }}</span>
            <span class="font-mono text-xs text-highlighted">{{ prompt.keyType }}</span>
          </div>
          <div v-if="changed" class="flex flex-col gap-1">
            <span class="label-caps text-dimmed">{{ t('hostKey.oldFingerprint') }}</span>
            <span class="font-mono text-xs break-all text-muted line-through">{{ prompt.oldFingerprint }}</span>
          </div>
          <div class="flex flex-col gap-1">
            <span class="label-caps text-dimmed">{{ t(changed ? 'hostKey.newFingerprint' : 'hostKey.fingerprint') }}</span>
            <span class="font-mono text-xs break-all text-highlighted">{{ prompt.fingerprint }}</span>
          </div>
        </div>
        <UAlert
          :color="changed ? 'error' : 'warning'"
          variant="soft"
          icon="i-lucide-triangle-alert"
          :description="t(changed ? 'hostKey.changedWarning' : 'hostKey.warning')"
        />
      </div>
    </template>

    <template #footer="{ close }">
      <UButton color="neutral" variant="subtle" :label="t('hostKey.cancel')" @click="close" />
      <UButton
        :color="changed ? 'error' : 'primary'"
        :loading="loading"
        :label="t(changed ? 'hostKey.trustChanged' : 'hostKey.trust')"
        @click="trust"
      />
    </template>
  </UModal>
</template>
