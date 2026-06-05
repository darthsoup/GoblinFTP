<script setup lang="ts">
const filesStore = useFilesStore()
const authStore = useAuthStore()
const { t } = useI18n()

const showHistory = computed(() => authStore.systemVars?.ui.showNavigationHistory ?? true)
</script>

<template>
  <nav class="flex items-center px-4 py-2 bg-muted border-b border-default font-mono text-xs overflow-x-auto whitespace-nowrap shrink-0">
    <!-- Back / forward history -->
    <div v-if="showHistory" class="flex items-center gap-0.5 mr-3 shrink-0">
      <button
        class="p-1 rounded text-muted enabled:hover:text-primary enabled:hover:bg-accented/50 disabled:opacity-40 transition-colors"
        :disabled="!filesStore.canGoBack"
        :title="t('breadcrumb.back')"
        @click="filesStore.goBack()"
      >
        <UIcon name="i-lucide-chevron-left" class="size-4 block" />
      </button>
      <button
        class="p-1 rounded text-muted enabled:hover:text-primary enabled:hover:bg-accented/50 disabled:opacity-40 transition-colors"
        :disabled="!filesStore.canGoForward"
        :title="t('breadcrumb.forward')"
        @click="filesStore.goForward()"
      >
        <UIcon name="i-lucide-chevron-right" class="size-4 block" />
      </button>
      <div class="h-4 w-px bg-accented mx-1.5" />
    </div>

    <UIcon name="i-lucide-monitor" class="size-4 text-primary mr-2 shrink-0" />

    <!-- Root -->
    <button
      class="text-muted hover:text-primary transition-colors shrink-0"
      :class="{ 'font-bold text-primary': filesStore.pathSegments.length === 0 }"
      @click="filesStore.navigate('/')"
    >
      /
      <span class="sr-only">{{ t('breadcrumb.root') }}</span>
    </button>

    <template v-for="(seg, i) in filesStore.pathSegments" :key="seg.path">
      <UIcon name="i-lucide-chevron-right" class="size-3 text-dimmed mx-1 shrink-0" />
      <button
        class="transition-colors truncate max-w-48"
        :class="i === filesStore.pathSegments.length - 1
          ? 'font-bold text-primary'
          : 'text-muted hover:text-primary'"
        @click="filesStore.navigate(seg.path)"
      >
        {{ seg.label }}
      </button>
    </template>
  </nav>
</template>
