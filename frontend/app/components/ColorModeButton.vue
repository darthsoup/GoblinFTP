<script setup lang="ts">
// Drop-in for <UColorModeButton>, but the flip animates via useColorModeTransition
// (circular reveal from the cursor). ClientOnly avoids an SSR hydration mismatch -
// the resolved mode isn't known until the client reads localStorage/system.
const colorMode = useColorMode()
const { t } = useI18n()
const { toggle } = useColorModeTransition()

const isDark = computed(() => colorMode.value === 'dark')
</script>

<template>
  <ClientOnly>
    <UButton
      :icon="isDark ? 'i-lucide-moon' : 'i-lucide-sun'"
      color="neutral"
      variant="ghost"
      :aria-label="t('settings.theme')"
      @click="toggle"
    />
    <template #fallback>
      <UButton icon="i-lucide-sun" color="neutral" variant="ghost" aria-hidden="true" />
    </template>
  </ClientOnly>
</template>
