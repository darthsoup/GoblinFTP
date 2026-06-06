<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const editorStore = useEditorStore()

useSessionChecker()

onMounted(async () => {
  await authStore.init()

  if (authStore.ssoAutoConnect) {
    await authStore.ssoConnect()
  }

  if (authStore.connected) {
    await filesStore.list(authStore.initialDirectory)
  }
})
</script>

<template>
  <div class="h-screen flex flex-col overflow-hidden bg-default text-default">
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
      <LoginForm />
    </template>

    <!-- Modals -->
    <RenameModal />
    <DeleteModal />
    <NewFolderModal />
    <NewFileModal />
    <PropertiesModal />
    <SessionExpiredModal />
  </div>
</template>
