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
    <!-- Connected: the file-browser app shell (fixed header/footer, scrolling body) -->
    <template v-if="authStore.connected">
      <AppHeader />
      <Breadcrumb />
      <EditorPane v-if="editorStore.hasOpenTabs" />
      <FileTable v-else />
      <UploadProgressPanel />
      <StatusBar />
    </template>

    <!-- Booting / login: centered content in a UMain + UContainer, with a
         real page footer for brand + pre-connect controls. -->
    <template v-else>
      <UMain class="flex-1 min-h-0 flex flex-col">
        <UContainer class="flex flex-1 flex-col items-center justify-center py-10">
          <div v-if="booting" class="flex items-center justify-center">
            <UIcon name="i-lucide-loader-circle" class="size-8 animate-spin text-primary" />
          </div>
          <LoginForm v-else />
        </UContainer>
      </UMain>

      <UFooter
        :ui="{
          root: 'shrink-0 border-t border-default bg-muted/50',
          container: 'px-4 py-0 h-12 flex items-center justify-between gap-3',
          left: 'mt-0 gap-x-1.5',
          right: 'mt-0 gap-x-1 justify-end',
        }"
      >
        <template #left>
          <span class="font-mono text-xs text-dimmed select-none">
            GoblinFTP {{ authStore.systemVars?.version ?? '' }}
          </span>
        </template>

        <template #right>
          <LanguageSelect variant="ghost" size="sm" class="font-mono" />
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
        </template>
      </UFooter>
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
