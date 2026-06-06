<script setup lang="ts">
import type { BreadcrumbItem } from '@nuxt/ui'

const filesStore = useFilesStore()
const authStore = useAuthStore()
const { t } = useI18n()

const showHistory = computed(() => authStore.systemVars?.ui.showNavigationHistory ?? true)

const items = computed<BreadcrumbItem[]>(() => [
  {
    'label': '/',
    'icon': 'i-lucide-monitor',
    'aria-label': t('breadcrumb.root'),
    'onClick': () => filesStore.navigate('/'),
  },
  ...filesStore.pathSegments.map(seg => ({
    label: seg.label,
    onClick: () => filesStore.navigate(seg.path),
  })),
])
</script>

<template>
  <nav class="flex items-center px-4 py-2 bg-muted border-b border-default overflow-x-auto whitespace-nowrap shrink-0">
    <!-- Back / forward history -->
    <div v-if="showHistory" class="flex items-center gap-0.5 mr-2 shrink-0">
      <UTooltip :text="t('breadcrumb.back')">
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-chevron-left"
          :disabled="!filesStore.canGoBack"
          :aria-label="t('breadcrumb.back')"
          @click="filesStore.goBack()"
        />
      </UTooltip>
      <UTooltip :text="t('breadcrumb.forward')">
        <UButton
          size="xs"
          color="neutral"
          variant="ghost"
          icon="i-lucide-chevron-right"
          :disabled="!filesStore.canGoForward"
          :aria-label="t('breadcrumb.forward')"
          @click="filesStore.goForward()"
        />
      </UTooltip>
      <USeparator orientation="vertical" class="h-4 mx-1.5" />
    </div>

    <UBreadcrumb
      :items="items"
      class="min-w-0"
      :ui="{
        list: 'gap-1',
        link: 'text-xs font-mono transition-colors cursor-pointer',
        linkLabel: 'truncate max-w-48',
        linkLeadingIcon: 'size-4 text-primary',
        separatorIcon: 'size-3 text-dimmed',
      }"
    />
  </nav>
</template>
