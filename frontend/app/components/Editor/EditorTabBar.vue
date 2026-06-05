<script setup lang="ts">
const props = defineProps<{ autoSave: boolean }>()
const emit = defineEmits<{ toggleAutoSave: [] }>()
const editorStore = useEditorStore()
const { t } = useI18n()
</script>

<template>
  <div class="flex items-center border-b border-default bg-muted overflow-x-auto shrink-0">
    <div
      v-for="tab in editorStore.tabs"
      :key="tab.id"
      class="flex items-center gap-1 px-3 py-1.5 text-sm font-mono cursor-pointer border-r border-default border-t-2 shrink-0 select-none transition-colors"
      :class="tab.id === editorStore.activeId
        ? 'bg-default text-primary border-t-primary'
        : 'border-t-transparent text-muted hover:bg-elevated hover:text-default'"
      @click="editorStore.setActive(tab.id)"
    >
      <span class="max-w-32 truncate">{{ tab.name }}</span>
      <span v-if="tab.content !== tab.savedContent" class="text-amber-400 leading-none" title="Unsaved changes">•</span>
      <UButton
        size="xs"
        color="neutral"
        variant="ghost"
        icon="i-lucide-x"
        class="-mr-1"
        :aria-label="t('editor.closeTab')"
        @click.stop="editorStore.closeTab(tab.id)"
      />
    </div>

    <div class="flex-1 min-w-4" />

    <div class="px-3 flex items-center gap-3 shrink-0">
      <label class="text-xs font-mono text-muted flex items-center gap-1.5 cursor-pointer whitespace-nowrap">
        <input
          type="checkbox"
          :checked="props.autoSave"
          class="rounded"
          @change="emit('toggleAutoSave')"
        >
        {{ t('editor.autoSave') }}
      </label>

      <UButton
        v-if="editorStore.activeId && !editorStore.activeTab?.loading"
        size="xs"
        color="primary"
        variant="subtle"
        icon="i-lucide-save"
        :loading="editorStore.activeTab?.saving"
        :disabled="editorStore.activeTab?.content === editorStore.activeTab?.savedContent"
        @click="editorStore.saveTab(editorStore.activeId!)"
      >
        {{ t('editor.save') }}
      </UButton>
    </div>
  </div>
</template>
