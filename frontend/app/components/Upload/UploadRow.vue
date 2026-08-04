<script setup lang="ts">
import type { UploadItem, UploadStatus } from '~/stores/upload'

const props = defineProps<{ item: UploadItem }>()
const emit = defineEmits<{
  cancel: [id: string]
  retry: [id: string]
}>()

const { t, locale } = useI18n()
const settingsStore = useSettingsStore()
const uploadStore = useUploadStore()
const { localizeError } = useErrorMessage()

const STATUS_CLASS: Record<UploadStatus, string> = {
  checking: 'text-dimmed',
  uploading: 'text-primary',
  queued: 'text-dimmed',
  done: 'text-muted',
  error: 'text-error',
  cancelled: 'text-dimmed',
  skipped: 'text-dimmed',
}

const ACTIVE: UploadStatus[] = ['checking', 'queued', 'uploading']
const RETRYABLE: UploadStatus[] = ['error', 'cancelled', 'skipped']

const path = computed(() => props.item.relativePath ?? props.item.file.name)
const isActive = computed(() => ACTIVE.includes(props.item.status))
const isRetryable = computed(() => RETRYABLE.includes(props.item.status))
const failure = computed(() => localizeError(props.item.errorCode ?? '', props.item.error ?? ''))

function fmt(n: number): string {
  return formatFileSize(n, settingsStore.sizeFormat, locale.value)
}

// Three states, deliberately distinct: null renders nothing (still measuring, or
// finalizing where any figure would be a guess about the server), 0 renders
// "Stalled", and a real rate renders alongside the ETA.
const telemetry = computed(() => {
  const item = props.item
  if (item.status !== 'uploading' || item.finalizing)
    return null
  const rate = uploadStore.rates[item.id] ?? null
  if (rate === null)
    return null
  if (rate === 0)
    return { kind: 'stalled' as const }
  return {
    kind: 'rate' as const,
    rate: formatRate(rate, settingsStore.sizeFormat, locale.value),
    eta: formatEta(etaSeconds(item.file.size - item.bytesUploaded, rate)),
  }
})
</script>

<template>
  <li class="group flex items-center gap-2.5 sm:gap-3 px-3 sm:px-4 py-2.5 border-b border-muted last:border-b-0 even:bg-elevated/40 text-xs">
    <div class="min-w-0 flex-1 flex flex-col gap-1.5">
      <!-- Name + byte counts (sizes shown from sm up; name always truncates) -->
      <div class="flex items-baseline gap-2">
        <span class="min-w-0 flex-1 truncate text-default" :title="path">{{ path }}</span>
        <span class="hidden sm:inline shrink-0 tabular-nums text-muted">
          <template v-if="item.status === 'uploading'">{{ fmt(item.bytesUploaded) }} / {{ fmt(item.file.size) }}</template>
          <template v-else>{{ fmt(item.file.size) }}</template>
        </span>
      </div>

      <!-- Progress bar + compact status (percent while uploading, else the label) —
           this line keeps every row informative at any width. -->
      <div class="flex items-center gap-2.5">
        <!-- null renders an indeterminate bar: during commit the bytes really
             are all sent but the server is still writing, so a percentage would
             be fabricated. -->
        <UProgress
          class="min-w-0 flex-1"
          size="sm"
          :model-value="item.finalizing ? null : item.progress"
          :color="item.status === 'error' ? 'error' : 'primary'"
        />
        <span v-if="telemetry" class="hidden sm:inline shrink-0 tabular-nums whitespace-nowrap text-dimmed">
          <template v-if="telemetry.kind === 'rate'">{{ telemetry.rate }} · {{ t('upload.eta', { time: telemetry.eta }) }}</template>
          <template v-else>{{ t('upload.stalled') }}</template>
        </span>
        <span class="shrink-0 tabular-nums font-medium whitespace-nowrap" :class="STATUS_CLASS[item.status]">
          <template v-if="item.finalizing">{{ t('upload.finalizing') }}</template>
          <template v-else-if="item.status === 'uploading'">{{ item.progress }}%</template>
          <template v-else>{{ t(`upload.status.${item.status}`) }}</template>
        </span>
      </div>

      <!-- Failure reason — always reachable now (was hidden below lg). -->
      <p v-if="item.status === 'error' && failure" class="truncate text-error" :title="failure">
        {{ failure }}
      </p>
    </div>

    <!-- Action: cancel while active, retry once failed/cancelled, check when done. -->
    <div class="shrink-0 w-7 flex justify-center">
      <UTooltip v-if="isActive" :text="t('upload.cancel')">
        <UButton
          size="xs"
          variant="ghost"
          color="neutral"
          icon="i-lucide-x"
          :aria-label="t('upload.cancel')"
          @click="emit('cancel', item.id)"
        />
      </UTooltip>
      <UTooltip v-else-if="isRetryable" :text="t('upload.retry')">
        <UButton
          size="xs"
          variant="ghost"
          color="neutral"
          icon="i-lucide-rotate-cw"
          :aria-label="t('upload.retry')"
          @click="emit('retry', item.id)"
        />
      </UTooltip>
      <UIcon v-else name="i-lucide-circle-check" class="size-4 text-primary" />
    </div>
  </li>
</template>
