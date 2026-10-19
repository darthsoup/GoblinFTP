<template>
  <AppHeader />
  <Breadcrumb tree-toggle :tree-open="desktop ? settings.directoryTreeVisible : drawerOpen" @toggle-tree="toggleTree" />
  <div class="flex min-h-0 flex-1 overflow-hidden">
    <aside v-if="desktop && settings.directoryTreeVisible" id="directory-tree" class="w-64 shrink-0 border-r border-default bg-muted/30">
      <DirectoryTree v-if="treeReady" />
    </aside>
    <FileTable class="min-w-0" />
  </div>
  <USlideover v-if="!desktop" v-model:open="drawerOpen" side="left" :title="$t('tree.title')" :ui="{ content: 'max-w-80', body: 'min-h-0 flex-1 p-0 sm:p-0' }">
    <template #body>
      <DirectoryTree v-if="treeReady && drawerOpen" id="directory-tree" @navigated="drawerOpen = false" />
    </template>
  </USlideover>
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
const settings = useSettingsStore()
const desktop = useMediaQuery('(min-width: 1024px)')
const drawerOpen = ref(false)
const treeReady = ref(false)
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
  treeReady.value = authStore.connected && !authStore.sessionLost
})

function toggleTree() {
  if (desktop.value)
    settings.directoryTreeVisible = !settings.directoryTreeVisible
  else
    drawerOpen.value = !drawerOpen.value
}

watch(desktop, () => {
  drawerOpen.value = false
})

// Keep ?path=<dir> in sync with the current directory (replace, not push: the
// store owns back/forward history) so a reload reopens the same folder.
watch(() => filesStore.currentPath, (path) => {
  if (route.query.path !== path)
    router.replace({ query: { ...route.query, path } })
})
</script>
