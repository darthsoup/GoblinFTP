<template>
  <div v-if="drafts.length" class="max-h-48 overflow-auto border-t border-default bg-elevated p-4 space-y-2">
    <div v-for="tab in drafts" :key="tab.id" class="flex flex-wrap items-center gap-3">
      <span class="text-sm text-muted"><bdi>{{ t('editor.detachedDraft', { host: displayName(tab.originHost ?? '') }) }}</bdi></span>
      <UButton icon="i-ph-download" color="neutral" variant="outline" @click="editorStore.exportDraft(tab.id)">
        {{ t('editor.exportDraft') }}: <bdi>{{ displayName(tab.name) }}</bdi>
      </UButton>
    </div>
  </div>
</template>

<script setup lang="ts">
const editorStore = useEditorStore()
const { t } = useI18n()
const drafts = computed(() => editorStore.tabs.filter(tab => tab.detached))
</script>
