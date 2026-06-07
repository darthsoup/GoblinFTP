<script setup lang="ts">
import type { NuxtError } from '#app'

// App-level fatal-error boundary (unhandled SPA errors / unknown routes).
// `clear` (UError default) renders a "back to home" button that calls
// clearError({ redirect: '/' }).
defineProps<{ error: NuxtError }>()
</script>

<template>
  <UApp>
    <UError
      :error="error"
      icon="i-lucide-server-off"
      :ui="{ root: 'min-h-screen bg-default text-default' }"
    />
  </UApp>
</template>
