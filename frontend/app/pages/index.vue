<template>
  <AppHeader />
  <Breadcrumb />
  <FileTable />
  <UploadProgressPanel />
  <!-- Branched here rather than inside StatusBar so the flex layout does not
       reserve the row when embedded. -->
  <StatusBar v-if="!embedded" />
</template>

<script setup lang="ts">
// The authenticated workspace. The auth middleware guarantees we only land here
// while connected; the layout's watcher routes back to /login on a drop.
const authStore = useAuthStore()
const filesStore = useFilesStore()
const route = useRoute()
const router = useRouter()
const { embedded } = useEmbed()

onMounted(async () => {
  // Restore the directory from the URL on reload; otherwise start at the
  // server's initial working directory.
  const queryPath = route.query.path
  const start = typeof queryPath === 'string' && queryPath
    ? queryPath
    : authStore.initialDirectory
  await filesStore.list(start)
})

// Keep ?path=<dir> in sync with the current directory (replace, not push: the
// store owns back/forward history) so a reload reopens the same folder.
watch(() => filesStore.currentPath, (path) => {
  if (route.query.path !== path)
    router.replace({ query: { ...route.query, path } })
})
</script>
