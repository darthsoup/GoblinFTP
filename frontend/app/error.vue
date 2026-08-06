<script setup lang="ts">
import type { NuxtError } from '#app'

// App-level fatal-error boundary (unhandled SPA errors / unknown routes). The
// layout is not mounted here, so the footer and its modal come along.
const props = defineProps<{ error: NuxtError }>()

const authStore = useAuthStore()

// `clear` (UError default) renders a "back to home" button that calls
// clearError({ redirect: '/' }). Without a way in, that would bounce right back.
const canReturn = computed(() => authStore.connected || !authStore.systemVars?.loginFormDisabled)
const icon = computed(() => props.error?.statusCode === 404 ? 'i-lucide-file-question-mark' : 'i-lucide-server-off')
</script>

<template>
  <UApp>
    <div class="min-h-screen flex flex-col bg-default text-default">
      <UError
        :error="error"
        :clear="canReturn"
        :icon="icon"
        :ui="{ root: 'flex-1 min-h-0' }"
      />
      <AppFooter />
    </div>

    <SettingsModal />
  </UApp>
</template>
