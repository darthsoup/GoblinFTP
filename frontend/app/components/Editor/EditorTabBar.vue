<script setup lang="ts">
const props = defineProps<{ autoSave: boolean }>()
const emit = defineEmits<{ toggleAutoSave: [] }>()
const editorStore = useEditorStore()
const { t } = useI18n()
</script>

<template>
  <div class="flex items-center border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 overflow-x-auto shrink-0">
    <div
      v-for="tab in editorStore.tabs"
      :key="tab.id"
      class="flex items-center gap-1 px-3 py-2 text-sm cursor-pointer border-r border-gray-200 dark:border-gray-700 shrink-0 select-none"
      :class="tab.id === editorStore.activeId
        ? 'bg-white dark:bg-gray-800 text-primary-600 dark:text-primary-400'
        : 'hover:bg-gray-100 dark:hover:bg-gray-800 text-gray-600 dark:text-gray-400'"
      @click="editorStore.setActive(tab.id)"
    >
      <span class="max-w-32 truncate">{{ tab.name }}</span>
      <span v-if="tab.content !== tab.savedContent" class="text-amber-500 leading-none" title="Unsaved changes">•</span>
      <UButton
        size="xs"
        variant="ghost"
        icon="i-heroicons-x-mark"
        class="-mr-1"
        :aria-label="t('editor.closeTab')"
        @click.stop="editorStore.closeTab(tab.id)"
      />
    </div>

    <div class="flex-1 min-w-4" />

    <div class="px-3 flex items-center gap-2 shrink-0">
      <label class="text-xs text-gray-500 flex items-center gap-1.5 cursor-pointer whitespace-nowrap">
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
        variant="ghost"
        icon="i-heroicons-arrow-down-tray"
        :loading="editorStore.activeTab?.saving"
        :disabled="editorStore.activeTab?.content === editorStore.activeTab?.savedContent"
        @click="editorStore.saveTab(editorStore.activeId!)"
      >
        {{ t('editor.save') }}
      </UButton>
    </div>
  </div>
</template>
