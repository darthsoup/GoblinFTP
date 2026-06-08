<script setup lang="ts">
// Shared shell for both routes (/login and /). Liveness polling lives here so
// it spans page changes; it self-skips when disconnected.
const authStore = useAuthStore()
const editorStore = useEditorStore()
const route = useRoute()

useSessionChecker()

// Warn on browser reload/close while the editor has unsaved buffers. Lives here
// (not on the editor page) so it still fires after returning to the browser with
// dirty tabs still held in the editor store.
function beforeUnload(e: BeforeUnloadEvent) {
  if (editorStore.hasDirty) {
    e.preventDefault()
    e.returnValue = ''
  }
}
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onUnmounted(() => window.removeEventListener('beforeunload', beforeUnload))

// Single source of truth for connected → route, independent of which page is
// mounted. The global auth middleware covers navigations; this covers in-place
// state flips (manual connect, SSO auto-connect, disconnect, session-expiry
// acknowledge) that the middleware can't see because no navigation occurs.
watch(() => authStore.connected, (connected) => {
  if (connected && route.path === '/login')
    navigateTo('/')
  else if (!connected && route.path !== '/login')
    navigateTo('/login')
})
</script>

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
  </div>
</template>
