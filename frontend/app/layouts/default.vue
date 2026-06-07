<script setup lang="ts">
// Shared shell for both routes (/login and /). Liveness polling lives here so
// it spans page changes; it self-skips when disconnected.
const authStore = useAuthStore()
const route = useRoute()

useSessionChecker()

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
    <RenameModal />
    <DeleteModal />
    <NewFolderModal />
    <NewFileModal />
    <PropertiesModal />
    <SettingsModal />
    <SessionExpiredModal />
  </div>
</template>
