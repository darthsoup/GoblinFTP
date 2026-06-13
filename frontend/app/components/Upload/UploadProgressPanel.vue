<script setup lang="ts">
import type { UploadStatus } from '~/stores/upload'

const uploadStore = useUploadStore()
const settingsStore = useSettingsStore()
const { t, locale } = useI18n()

const collapsed = ref(false)

const STATUS_CLASS: Record<UploadStatus, string> = {
  uploading: 'text-primary font-medium',
  queued: 'text-dimmed',
  done: 'text-muted',
  error: 'text-error font-medium',
  cancelled: 'text-dimmed',
}

function statusLabel(status: UploadStatus): string {
  return t(`upload.status.${status}`)
}

function formatBytes(n: number): string {
  return formatFileSize(n, settingsStore.sizeFormat, locale.value)
}
</script>

<template>
  <div
    v-if="uploadStore.items.length > 0"
    class="flex flex-col border-t border-default bg-elevated/40 shrink-0"
  >
    <!-- Header -->
    <div
      class="flex items-center justify-between px-4 h-10 bg-muted shrink-0"
      :class="{ 'border-b border-default': !collapsed }"
    >
      <div class="flex items-center gap-2 select-none">
        <UTooltip :text="t('upload.toggle')">
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            icon="i-lucide-chevron-down"
            :aria-label="t('upload.toggle')"
            :ui="{ leadingIcon: ['transition-transform', collapsed ? '-rotate-90' : ''] }"
            @click="collapsed = !collapsed"
          />
        </UTooltip>
        <UIcon name="i-lucide-arrow-up-down" class="size-4 text-primary" />
        <span class="label-caps text-highlighted">{{ t('upload.queue') }}</span>
        <UBadge color="primary" variant="soft" size="sm" class="font-bold rounded-full">
          {{ uploadStore.items.length }}
        </UBadge>
      </div>

      <div class="flex items-center gap-1">
        <UButton
          v-if="uploadStore.hasActive"
          size="xs"
          variant="ghost"
          color="error"
          icon="i-lucide-circle-pause"
          @click="uploadStore.cancelAll()"
        >
          {{ t('upload.cancelAll') }}
        </UButton>
        <UButton
          v-else
          size="xs"
          variant="ghost"
          color="neutral"
          icon="i-lucide-list-x"
          @click="uploadStore.clearDone()"
        >
          {{ t('upload.clear') }}
        </UButton>
      </div>
    </div>

    <!-- Item list -->
    <div v-show="!collapsed" class="max-h-44 overflow-y-auto">
      <div
        v-for="item in uploadStore.items"
        :key="item.id"
        class="flex items-center gap-4 px-4 py-2 border-b border-muted last:border-b-0 even:bg-elevated/40 text-xs"
      >
        <!-- Name + progress -->
        <div class="flex flex-col gap-1.5 flex-1 min-w-0 sm:max-w-md">
          <span class="truncate text-default" :title="item.file.name">{{ item.file.name }}</span>
          <UProgress
            :model-value="item.progress"
            size="sm"
            :color="item.status === 'error' ? 'error' : 'primary'"
          />
        </div>

        <!-- Status -->
        <span class="w-24 shrink-0 hidden sm:inline" :class="STATUS_CLASS[item.status]">
          {{ statusLabel(item.status) }}
        </span>

        <!-- Bytes -->
        <span class="w-36 shrink-0 text-right text-muted hidden md:inline">
          <template v-if="item.status === 'uploading'">
            {{ formatBytes(item.bytesUploaded) }} / {{ formatBytes(item.file.size) }}
          </template>
          <template v-else>
            {{ formatBytes(item.file.size) }}
          </template>
        </span>

        <!-- Error message (mobile-friendly inline) -->
        <span
          v-if="item.status === 'error' && item.error"
          class="text-error truncate max-w-48 hidden lg:inline"
          :title="item.error"
        >
          {{ item.error }}
        </span>

        <!-- Action / state icon -->
        <span class="w-7 shrink-0 flex justify-center">
          <UTooltip v-if="item.status === 'uploading' || item.status === 'queued'" :text="t('upload.cancel')">
            <UButton
              size="xs"
              variant="ghost"
              color="neutral"
              icon="i-lucide-x"
              :aria-label="t('upload.cancel')"
              @click="uploadStore.cancelItem(item.id)"
            />
          </UTooltip>
          <UIcon v-else-if="item.status === 'done'" name="i-lucide-circle-check" class="size-4 text-primary" />
          <UIcon v-else-if="item.status === 'error'" name="i-lucide-circle-x" class="size-4 text-error" />
          <UIcon v-else name="i-lucide-circle-minus" class="size-4 text-dimmed" />
        </span>
      </div>
    </div>
  </div>
</template>
