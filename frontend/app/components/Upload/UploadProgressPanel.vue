<script setup lang="ts">
const uploadStore = useUploadStore()
const { t } = useI18n()

const collapsed = ref(false)

function toggle() {
  collapsed.value = !collapsed.value
}

// Overall progress across still-active items, so a collapsed queue still reports
// movement (null when nothing is in flight → the header indicator is hidden).
const overall = computed(() => {
  const active = uploadStore.items.filter(i => i.status === 'uploading' || i.status === 'queued')
  if (active.length === 0)
    return null
  const total = active.reduce((sum, i) => sum + i.file.size, 0)
  const done = active.reduce((sum, i) => sum + i.bytesUploaded, 0)
  return total > 0 ? Math.round((done / total) * 100) : 0
})
</script>

<template>
  <section
    v-if="uploadStore.items.length > 0"
    class="flex flex-col border-t border-default bg-elevated/40 shrink-0"
  >
    <!-- Header -->
    <div
      class="flex items-center justify-between gap-2 px-2 sm:px-4 h-10 bg-elevated shrink-0"
      :class="{ 'border-b border-default': !collapsed }"
    >
      <div class="flex items-center gap-1.5 sm:gap-2 min-w-0 select-none">
        <UTooltip :text="t('upload.toggle')">
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            icon="i-lucide-chevron-down"
            :aria-label="t('upload.toggle')"
            :aria-expanded="!collapsed"
            :ui="{ leadingIcon: ['transition-transform', collapsed ? '-rotate-90' : ''] }"
            @click="toggle"
          />
        </UTooltip>
        <UIcon name="i-lucide-arrow-up-down" class="size-4 text-primary shrink-0 hidden sm:block" />
        <span class="label-caps text-highlighted truncate">{{ t('upload.queue') }}</span>
        <UBadge color="primary" variant="soft" size="sm" class="font-bold rounded-full shrink-0">
          {{ uploadStore.items.length }}
        </UBadge>

        <!-- Aggregate progress — only while collapsed and something is uploading. -->
        <span v-if="collapsed && overall !== null" class="flex items-center gap-2 min-w-0 pl-1">
          <UProgress class="w-14 sm:w-28" size="sm" :model-value="overall" />
          <span class="text-xs tabular-nums text-muted shrink-0">{{ overall }}%</span>
        </span>
      </div>

      <div class="flex items-center gap-1 shrink-0">
        <UButton
          v-if="uploadStore.hasActive"
          size="xs"
          variant="ghost"
          color="error"
          icon="i-lucide-circle-pause"
          @click="uploadStore.cancelAll()"
        >
          <span class="hidden sm:inline">{{ t('upload.cancelAll') }}</span>
        </UButton>
        <UButton
          v-else
          size="xs"
          variant="ghost"
          color="neutral"
          icon="i-lucide-list-x"
          @click="uploadStore.clearDone()"
        >
          <span class="hidden sm:inline">{{ t('upload.clear') }}</span>
        </UButton>
      </div>
    </div>

    <!-- Item list -->
    <ul v-show="!collapsed" class="max-h-44 sm:max-h-56 overflow-y-auto">
      <UploadRow
        v-for="item in uploadStore.items"
        :key="item.id"
        :item="item"
        @cancel="uploadStore.cancelItem"
        @retry="uploadStore.retryItem"
      />
    </ul>
  </section>
</template>
