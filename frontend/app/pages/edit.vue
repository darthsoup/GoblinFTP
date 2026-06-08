<script setup lang="ts">
// The editor route. The file browser (and its breadcrumb) is NOT mounted here,
// so there is no folder navigation under the editor. The open file lives in
// ?path=<file> (deep-linkable + reload-restorable); tabs persist in the editor
// store, so you can return via the header's "Editor" button.
const editorStore = useEditorStore()
const authStore = useAuthStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

function parentDir(p: string): string {
  const trimmed = p.replace(/\/+$/, '')
  const i = trimmed.lastIndexOf('/')
  return i <= 0 ? '/' : trimmed.slice(0, i)
}

const filePath = computed(() => editorStore.activeTab?.path ?? '')

async function openFromQuery() {
  const p = route.query.path
  if (typeof p === 'string' && p)
    await editorStore.openFile(p)
}

// Opening another file from the browser navigates here with a new ?path.
watch(() => route.query.path, (p) => {
  if (typeof p === 'string' && p && editorStore.activeTab?.path !== p)
    editorStore.openFile(p)
})

// Keep ?path in sync with the active tab so a reload reopens it.
watch(() => editorStore.activeTab?.path, (p) => {
  if (p && route.query.path !== p)
    router.replace({ query: { ...route.query, path: p } })
})

// No tabs left → return to the file browser. Guarded by `connected` so a
// disconnect (which empties the editor and routes to /login via the layout
// watcher) doesn't also fire a competing navigation to /.
watch(() => editorStore.hasOpenTabs, (open) => {
  if (!open && authStore.connected)
    navigateTo('/')
})

async function backToFiles() {
  await navigateTo({ path: '/', query: filePath.value ? { path: parentDir(filePath.value) } : {} })
}

onMounted(async () => {
  await openFromQuery()
  if (!editorStore.hasOpenTabs)
    navigateTo('/')
})
</script>

<template>
  <AppHeader />

  <div class="flex items-center gap-2 px-4 py-2 bg-muted border-b border-default shrink-0">
    <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-arrow-left" @click="backToFiles">
      {{ t('editor.backToFiles') }}
    </UButton>
    <USeparator orientation="vertical" class="h-4" />
    <span class="font-mono text-xs text-muted truncate">{{ filePath }}</span>
  </div>

  <EditorPane />
  <StatusBar />
</template>
