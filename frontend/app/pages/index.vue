<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const editorStore = useEditorStore()
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const { t, locale, setLocale } = useI18n()

useSessionChecker()

// Spinner gate for the initial boot / SSO auto-connect only — a manual connect
// must NOT swap the login form for the spinner, or its error + typed input are
// lost when the form remounts.
const booting = ref(true)

// Maps a backend ?sso_error=<reason> redirect (bad/expired/replayed link) to a
// translated message. Unknown reasons fall back to the generic string.
function ssoErrorMessage(reason: string): string {
  const key = `sso.error${reason.charAt(0).toUpperCase()}${reason.slice(1)}`
  const msg = t(key)
  return msg === key ? t('sso.errorGeneric') : msg
}

onMounted(async () => {
  try {
    // Surface a bad/expired SSO link redirected here by the backend, then strip
    // the param so a refresh doesn't re-show it.
    const params = new URLSearchParams(window.location.search)
    const ssoError = params.get('sso_error')

    await authStore.init()

    // Language precedence: explicit user choice > admin default > en
    const adminLang = authStore.systemVars?.language
    const preferred = settingsStore.language
      ?? (adminLang === 'en' || adminLang === 'de' ? adminLang : undefined)
    if (preferred && preferred !== locale.value)
      await setLocale(preferred)

    if (ssoError) {
      authStore.error = ssoErrorMessage(ssoError)
      window.history.replaceState(null, '', window.location.pathname)
    }

    if (authStore.ssoAutoConnect) {
      await authStore.ssoConnect()
    }

    if (authStore.connected) {
      await filesStore.list(authStore.initialDirectory)
    }
  }
  finally {
    booting.value = false
  }
})
</script>

<template>
  <div class="relative h-screen flex flex-col overflow-hidden bg-default text-default">
    <template v-if="authStore.connected">
      <AppHeader />
      <Breadcrumb />
      <EditorPane v-if="editorStore.hasOpenTabs" />
      <FileTable v-else />
      <UploadProgressPanel />
      <StatusBar />
    </template>
    <template v-else-if="booting">
      <div class="flex items-center justify-center flex-1">
        <UIcon name="i-lucide-loader-circle" class="size-8 animate-spin text-primary" />
      </div>
    </template>
    <template v-else>
      <!-- Settings reachable before connecting (language/theme) -->
      <div class="absolute top-3 right-3 z-10">
        <UTooltip :text="t('header.settings')">
          <UButton
            color="neutral"
            variant="ghost"
            icon="i-lucide-settings"
            :aria-label="t('header.settings')"
            @click="modalStore.open('settings')"
          />
        </UTooltip>
      </div>
      <LoginForm />
    </template>

    <!-- Modals -->
    <RenameModal />
    <DeleteModal />
    <NewFolderModal />
    <NewFileModal />
    <PropertiesModal />
    <SettingsModal />
    <SessionExpiredModal />
  </div>
</template>
