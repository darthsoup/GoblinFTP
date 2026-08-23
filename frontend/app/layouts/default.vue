<template>
  <div class="relative h-screen flex flex-col overflow-hidden bg-default text-default">
    <slot />

    <!-- Global overlays (each renders only when its modalStore state is active) -->
    <DeleteModal />
    <NewFolderModal />
    <NewFileModal />
    <PropertiesModal />
    <SettingsModal />
    <SessionExpiredModal />
    <ConfirmModal />
    <ShortcutsModal />
    <PasteConflictModal />
    <UploadConflictModal />
    <EditorConflictModal />
  </div>
</template>

<script setup lang="ts">
// Shared shell for both routes (/login and /). Liveness polling lives here so
// it spans page changes; it self-skips when disconnected.
const authStore = useAuthStore()
const editorStore = useEditorStore()
const route = useRoute()

useSessionChecker()

// Warn on reload/close while the editor has unsaved buffers. Lives here, not on
// the editor page, so it still fires after returning to the file browser.
useEventListener(window, 'beforeunload', (e: BeforeUnloadEvent) => {
  if (editorStore.hasDirty) {
    e.preventDefault()
    e.returnValue = ''
  }
})

// Single source of truth for connected → route. The global auth middleware
// covers navigations; this covers in-place flips, which it cannot see.
watch(() => authStore.connected, (connected) => {
  if (connected && route.path === '/login')
    navigateTo('/')
  else if (!connected && route.path !== '/login')
    navigateTo('/login')
})
</script>
