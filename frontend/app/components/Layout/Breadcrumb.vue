<script setup lang="ts">
import type { BreadcrumbItem } from '@nuxt/ui'

const filesStore = useFilesStore()
const authStore = useAuthStore()
const { t } = useI18n()

const showHistory = computed(() => authStore.systemVars?.ui.showNavigationHistory ?? true)

// On phones a deep path is collapsed to: root › … (parent) › current.
const isNarrow = useMediaQuery('(max-width: 639px)')

const items = computed<BreadcrumbItem[]>(() => {
  const root: BreadcrumbItem = {
    'label': '/',
    'icon': 'i-lucide-server',
    'aria-label': t('breadcrumb.root'),
    'onClick': () => filesStore.navigate('/'),
  }
  const segs = filesStore.pathSegments
  // The last segment carries a folder glyph; with color="neutral" UBreadcrumb
  // renders it highlighted and semibold against the muted ancestors.
  let segItems: BreadcrumbItem[] = segs.map((seg, i) => ({
    label: seg.label,
    ...(i === segs.length - 1 ? { icon: 'i-lucide-folder-open' } : {}),
    onClick: () => filesStore.navigate(seg.path),
  }))

  if (isNarrow.value && segs.length > 2) {
    const parent = segs[segs.length - 2]!
    const current = segItems[segItems.length - 1]!
    segItems = [
      { 'label': '…', 'aria-label': parent.label, 'onClick': () => filesStore.navigate(parent.path) },
      current,
    ]
  }

  return [root, ...segItems]
})
</script>

<template>
  <nav class="flex items-center gap-2 px-4 h-11 bg-elevated border-b border-default overflow-x-auto whitespace-nowrap shrink-0">
    <template v-if="showHistory">
      <UFieldGroup size="sm" class="shrink-0">
        <UTooltip :text="t('breadcrumb.back')">
          <UButton
            color="neutral"
            variant="subtle"
            icon="i-lucide-chevron-left"
            :disabled="!filesStore.canGoBack"
            :aria-label="t('breadcrumb.back')"
            @click="filesStore.goBack()"
          />
        </UTooltip>
        <UTooltip :text="t('breadcrumb.forward')">
          <UButton
            color="neutral"
            variant="subtle"
            icon="i-lucide-chevron-right"
            :disabled="!filesStore.canGoForward"
            :aria-label="t('breadcrumb.forward')"
            @click="filesStore.goForward()"
          />
        </UTooltip>
      </UFieldGroup>
      <USeparator orientation="vertical" class="h-5 shrink-0" />
    </template>

    <UBreadcrumb
      :items="items"
      color="neutral"
      class="min-w-0"
      :ui="{
        list: 'gap-1',
        link: 'rounded-sm px-1.5 py-0.5 hover:bg-accented/60 transition-colors duration-150 cursor-pointer',
        linkLabel: 'truncate max-w-32 sm:max-w-48',
        // Inherit the segment's text colour (currentColor) so the active glyph is
        // highlighted like its label and ancestors stay muted, with no accent.
        linkLeadingIcon: 'size-4 shrink-0',
        separatorIcon: 'size-3.5 text-dimmed',
      }"
    />
  </nav>
</template>
