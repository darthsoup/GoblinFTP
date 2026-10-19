<template>
  <div class="min-w-0">
    <span v-if="label || (result && (loading || entry.status === 'error'))" class="flex items-center gap-1" :title="explanation || undefined" :aria-label="result && loading ? `${label}: ${t('directorySize.calculating')}` : explanation ? `${label}: ${explanation}` : undefined" role="status">
      <span class="truncate">{{ label }}</span>
      <UIcon v-if="result && loading" name="i-ph-spinner-gap" class="size-3 shrink-0 animate-spin" :aria-label="t('directorySize.calculating')" />
      <UIcon v-else-if="result && entry.status === 'error'" name="i-ph-warning" class="size-3 shrink-0 text-error" :aria-label="explanation" />
    </span>
    <template v-if="details">
      <p v-if="result && loading" class="mt-1 text-xs text-muted">
        {{ t('directorySize.calculating') }}
      </p>
      <div v-if="result" class="mt-1 space-y-1 text-xs text-muted">
        <p>{{ t('directorySize.counts', { files: result.fileCount.toLocaleString(locale), directories: result.directoryCount.toLocaleString(locale) }) }}</p>
        <p v-if="!result.complete">
          {{ reason }}
        </p>
        <p v-if="result.skippedCount">
          {{ t('directorySize.skipped', { count: result.skippedCount.toLocaleString(locale) }) }}
        </p>
      </div>
      <p v-if="entry.status === 'error'" class="mt-1 text-xs text-error">
        {{ explanation }}
      </p>
      <UButton type="button" size="sm" color="neutral" variant="subtle" class="mt-2" :icon="loading ? 'i-ph-x' : 'i-ph-calculator'" :label="actionLabel" @click.stop="act" />
    </template>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{ path: string, details?: boolean }>()
const { t, locale } = useI18n()
const { entry, result, loading, explanation, reason, label, actionLabel, act } = useDirectorySize(() => props.path)
</script>
