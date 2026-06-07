<script setup lang="ts">
// The authenticated workspace. The global auth middleware guarantees we only
// land here while connected, and the layout's connected-watcher routes back to
// /login if the connection drops.
const authStore = useAuthStore()
const filesStore = useFilesStore()
const editorStore = useEditorStore()
const route = useRoute()
const router = useRouter()

onMounted(async () => {
  // Restore the directory from the URL on reload; otherwise start at the
  // server's initial working directory.
  const queryPath = route.query.path
  const start = typeof queryPath === 'string' && queryPath
    ? queryPath
    : authStore.initialDirectory
  await filesStore.list(start)
})

// Keep ?path=<dir> in sync with the current directory (replace, not push — the
// store owns back/forward history) so a reload reopens the same folder.
watch(() => filesStore.currentPath, (path) => {
  if (route.query.path !== path)
    router.replace({ query: { ...route.query, path } })
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
