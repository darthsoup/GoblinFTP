<template>
  <div class="flex flex-1 min-h-0 flex-col">
    <div v-if="authStore.ssoPending" class="flex flex-1 items-center justify-center p-4">
      <div class="w-full max-w-md rounded-lg border border-default bg-elevated p-8 space-y-5">
        <h1 class="text-xl font-semibold text-highlighted">
          {{ t('sso.confirmTitle') }}
        </h1>
        <p class="text-muted">
          <bdi>{{ displayName(authStore.ssoPending.protocol.toUpperCase()) }} {{ displayName(authStore.ssoPending.host) }}:{{ authStore.ssoPending.port }}</bdi>
        </p>
        <p><bdi>{{ displayName(authStore.ssoPending.username) }}</bdi></p>
        <p v-if="authStore.error" class="text-error">
          {{ authStore.error }}
        </p>
        <div class="flex gap-3">
          <UButton :loading="authStore.loading" @click="authStore.ssoConnect()">
            {{ t('login.connect') }}
          </UButton>
          <UButton variant="outline" :disabled="authStore.loading" @click="cancelSSO">
            {{ t('hostKey.cancel') }}
          </UButton>
        </div>
      </div>
    </div>
    <LoginScreen v-else :booting="booting" />
    <HostKeyVerifyModal v-if="authStore.ssoPending" />
    <DetachedDrafts />
  </div>
</template>

<script setup lang="ts">
const authStore = useAuthStore()
const { t } = useI18n()
const booting = ref(true)
watchEffect(() => {
  if (!booting.value && authStore.connected && !authStore.ssoPending)
    void navigateTo('/')
})

async function cancelSSO() {
  await authStore.cancelSSO()
  if (authStore.connected)
    await navigateTo('/')
}

onMounted(async () => {
  try {
    await authStore.loadSSOPending()
    const reason = new URLSearchParams(window.location.search).get('sso_error')
    if (reason) {
      const key = `sso.error${reason.charAt(0).toUpperCase()}${reason.slice(1)}`
      const message = t(key)
      authStore.error = message === key ? t('sso.errorGeneric') : message
    }
    window.history.replaceState(null, '', window.location.pathname)
    if (!authStore.ssoPending && !authStore.connected && authStore.systemVars?.loginFormDisabled)
      showError(createError({ statusCode: 404, statusMessage: t('error.notFound') }))
  }
  finally { booting.value = false }
})
</script>
