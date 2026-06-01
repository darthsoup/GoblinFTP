<script setup lang="ts">
const uploadStore = useUploadStore()
const { t } = useI18n()

const statusIcon: Record<string, string> = {
  done: 'i-heroicons-check-circle',
  error: 'i-heroicons-x-circle',
  cancelled: 'i-heroicons-minus-circle',
  uploading: '',
  queued: '',
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}
</script>

<template>
  <div
    v-if="uploadStore.items.length > 0"
    class="fixed bottom-4 right-4 w-80 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg z-50 overflow-hidden"
  >
    <!-- Header -->
    <div class="flex items-center justify-between px-3 py-2 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900">
      <span class="text-sm font-medium">
        {{ t('upload.title', { n: uploadStore.items.length }) }}
      </span>
      <div class="flex items-center gap-1">
        <UButton
          v-if="uploadStore.hasActive"
          size="xs"
          variant="ghost"
          color="error"
          @click="uploadStore.cancelAll()"
        >
          {{ t('upload.cancelAll') }}
        </UButton>
        <UButton
          v-else
          size="xs"
          variant="ghost"
          @click="uploadStore.clearDone()"
        >
          {{ t('upload.clear') }}
        </UButton>
      </div>
    </div>

    <!-- Item list -->
    <div class="max-h-48 overflow-y-auto divide-y divide-gray-100 dark:divide-gray-700">
      <div
        v-for="item in uploadStore.items"
        :key="item.id"
        class="px-3 py-2"
      >
        <div class="flex items-center justify-between gap-2">
          <span class="text-sm truncate flex-1" :title="item.file.name">{{ item.file.name }}</span>
          <UButton
            v-if="item.status === 'uploading' || item.status === 'queued'"
            size="xs"
            variant="ghost"
            icon="i-heroicons-x-mark"
            :aria-label="t('upload.cancel')"
            @click="uploadStore.cancelItem(item.id)"
          />
          <UIcon
            v-else-if="item.status === 'done'"
            :name="statusIcon.done"
            class="text-green-500 shrink-0"
          />
          <UIcon
            v-else-if="item.status === 'error'"
            :name="statusIcon.error"
            class="text-red-500 shrink-0"
          />
          <UIcon
            v-else-if="item.status === 'cancelled'"
            :name="statusIcon.cancelled"
            class="text-gray-400 shrink-0"
          />
        </div>

        <!-- Progress bar for active uploads -->
        <UProgress
          v-if="item.status === 'uploading'"
          :value="item.progress"
          class="mt-1"
          size="sm"
        />

        <!-- Bytes transferred for active uploads -->
        <div
          v-if="item.status === 'uploading'"
          class="text-xs text-gray-500 mt-0.5"
        >
          {{ formatBytes(item.bytesUploaded) }} / {{ formatBytes(item.file.size) }}
        </div>

        <!-- Queued label -->
        <div
          v-if="item.status === 'queued'"
          class="text-xs text-gray-400 mt-0.5"
        >
          {{ t('upload.queued') }}
        </div>

        <!-- Error message -->
        <p
          v-if="item.status === 'error'"
          class="text-xs text-red-500 mt-0.5 truncate"
          :title="item.error"
        >
          {{ item.error }}
        </p>
      </div>
    </div>
  </div>
</template>
