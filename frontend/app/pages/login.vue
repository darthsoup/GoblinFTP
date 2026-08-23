<script setup lang="ts">
// The unauthenticated route. Session and language bootstrap live in
// plugins/auth.client.ts; this page only handles the SSO link landing.
const authStore = useAuthStore()
const { t } = useI18n()

// Spinner only while an SSO auto-connect is in flight; a plain login load shows
// the form immediately (the session was already resolved by the auth plugin).
const booting = ref(authStore.ssoAutoConnect)

// With the login form disabled a signed-out visitor has no way in, so the route
// is a dead end: present it as a missing page instead of a session explanation.
const loginFormDisabled = computed(() => authStore.systemVars?.loginFormDisabled ?? false)
watchEffect(() => {
  if (!booting.value && !authStore.connected && loginFormDisabled.value)
    showError(createError({ statusCode: 404, statusMessage: t('error.notFound') }))
})

// Maps a backend ?sso_error=<reason> redirect to a translated message.
function ssoErrorMessage(reason: string): string {
  const key = `sso.error${reason.charAt(0).toUpperCase()}${reason.slice(1)}`
  const msg = t(key)
  return msg === key ? t('sso.errorGeneric') : msg
}

onMounted(async () => {
  // Surface a bad/expired SSO link redirected here by the backend, then strip
  // the param so a refresh doesn't re-show it.
  const ssoError = new URLSearchParams(window.location.search).get('sso_error')
  if (ssoError) {
    authStore.error = ssoErrorMessage(ssoError)
    window.history.replaceState(null, '', window.location.pathname)
  }

  // Finalize an SSO link (the session carries a pending connection). On success
  // `connected` flips and the layout watcher routes to the workspace.
  if (authStore.ssoAutoConnect) {
    try {
      await authStore.ssoConnect()
    }
    finally {
      booting.value = false
    }
  }
})
</script>

<template>
  <LoginScreen :booting="booting" />
</template>
