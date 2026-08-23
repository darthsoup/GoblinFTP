<template>
  <UMain class="flex-1 min-h-0 flex flex-col">
    <UContainer class="flex flex-1 flex-col items-center justify-center py-10">
      <div v-if="booting" class="flex items-center justify-center">
        <UIcon name="i-lucide-loader-circle" class="size-10 animate-spin text-primary" />
      </div>
      <LoginForm v-else />
    </UContainer>
  </UMain>

  <AppFooter />

  <HostKeyVerifyModal />
</template>

<script setup lang="ts">
// pages/login.vue owns `booting` and the SSO landing; this unauthenticated view
// is purely presentational.
defineProps<{ booting: boolean }>()
</script>
