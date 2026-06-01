<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const editorStore = useEditorStore()

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
  <div class="flex flex-col min-h-screen">
    <template v-if="authStore.connected">
      <AppHeader />
      <Breadcrumb />
      <EditorPane v-if="editorStore.hasOpenTabs" />
      <FileTable v-else />
    </template>
    <template v-else-if="authStore.loading">
      <div class="flex items-center justify-center min-h-screen">
        <UIcon name="i-heroicons-arrow-path" class="w-8 h-8 animate-spin text-primary-500" />
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
    <ChmodModal />
    <PropertiesModal />

    <!-- Upload progress -->
    <UploadProgressPanel />
  </div>
</template>
