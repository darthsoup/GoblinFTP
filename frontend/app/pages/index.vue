<script setup lang="ts">
// The authenticated workspace. The global auth middleware guarantees we only
// land here while connected, and the layout's connected-watcher routes back to
// /login if the connection drops.
const authStore = useAuthStore()
const filesStore = useFilesStore()
const editorStore = useEditorStore()

onMounted(async () => {
  await filesStore.list(authStore.initialDirectory)
})
</script>

<template>
  <AppHeader />
  <Breadcrumb />
  <EditorPane v-if="editorStore.hasOpenTabs" />
  <FileTable v-else />
  <UploadProgressPanel />
  <StatusBar />
</template>
