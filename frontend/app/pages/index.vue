<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const editorStore = useEditorStore()
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const { t, locale, setLocale } = useI18n()

useSessionChecker()

onMounted(async () => {
  await authStore.init()

  // Language precedence: explicit user choice > admin default > en
  const adminLang = authStore.systemVars?.language
  const preferred = settingsStore.language
    ?? (adminLang === 'en' || adminLang === 'de' ? adminLang : undefined)
  if (preferred && preferred !== locale.value)
    await setLocale(preferred)

  if (authStore.ssoAutoConnect) {
    await authStore.ssoConnect()
  }

  if (authStore.connected) {
    await filesStore.list(authStore.initialDirectory)
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
    <template v-else-if="authStore.loading">
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
