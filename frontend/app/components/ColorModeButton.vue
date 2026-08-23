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

<script setup lang="ts">
// Drop-in for <UColorModeButton> with an animated flip. ClientOnly avoids a
// hydration mismatch: the resolved mode is known only once the client reads it.
const colorMode = useColorMode()
const { t } = useI18n()
const { toggle } = useColorModeTransition()

const isDark = computed(() => colorMode.value === 'dark')
</script>
