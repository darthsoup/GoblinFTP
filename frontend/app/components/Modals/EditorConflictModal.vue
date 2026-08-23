<template>
  <UModal
    :open="modalStore.active === 'editorConflict'"
    :title="deleted ? t('editor.conflict.deletedTitle') : t('editor.conflict.title')"
    @update:open="(v: boolean) => { if (!v) modalStore.resolveEditorConflict('cancel') }"
  >
    <template #title>
      <UIcon name="i-lucide-triangle-alert" class="size-5 text-warning" />
      {{ deleted ? t('editor.conflict.deletedTitle') : t('editor.conflict.title') }}
    </template>

    <template #body>
      <p v-if="info" class="text-sm text-muted">
        {{ deleted
          ? t('editor.conflict.deletedMessage', { name: info.name })
          : t('editor.conflict.message', { name: info.name }) }}
      </p>
      <p v-if="baseline" class="mt-3 rounded border border-default bg-muted/40 px-3 py-2 text-xs text-dimmed">
        {{ baseline }}
      </p>
    </template>

    <template #footer>
      <UButton
        color="neutral"
        variant="ghost"
        :label="t('editor.keepEditing')"
        @click="modalStore.resolveEditorConflict('cancel')"
      />
      <UButton
        v-if="!deleted"
        color="neutral"
        variant="outline"
        :label="t('editor.conflict.reload')"
        @click="modalStore.resolveEditorConflict('reload')"
      />
      <UButton
        color="error"
        :label="t('editor.conflict.overwrite')"
        @click="modalStore.resolveEditorConflict('overwrite')"
      />
    </template>
  </UModal>
</template>

<script setup lang="ts">
// Raised when a save was refused because the file changed or vanished. Driven by
// modalStore.editorConflict(); backdrop/Esc resolves as the safe 'cancel'.
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const { t, locale } = useI18n()

const info = computed(() => modalStore.editorConflictInfo)
const deleted = computed(() => info.value?.kind === 'deleted')

const baseline = computed(() => {
  const i = info.value
  if (!i || i.size === undefined || !i.modified)
    return null
  return t('editor.conflict.baseline', {
    size: formatFileSize(i.size, settingsStore.sizeFormat, locale.value),
    date: formatFileDate(i.modified, settingsStore.dateFormat, locale.value),
  })
})
</script>
