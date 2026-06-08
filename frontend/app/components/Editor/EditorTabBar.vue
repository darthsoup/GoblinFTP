<script setup lang="ts">
import type { EditorTab } from '~/stores/editor'

const editorStore = useEditorStore()
const settingsStore = useSettingsStore()
const modalStore = useModalStore()
const authStore = useAuthStore()
const { t } = useI18n()

const viewOnly = computed(() => authStore.systemVars?.editor?.viewOnly ?? false)

// Confirm before discarding a tab with unsaved changes (this is also the only
// way out of the editor, so it must not silently drop work).
async function requestClose(tab: EditorTab) {
  if (tab.content !== tab.savedContent) {
    const result = await modalStore.confirm({
      title: t('editor.unsavedTitle'),
      message: t('editor.confirmCloseMessage', { name: tab.name }),
      saveLabel: viewOnly.value ? undefined : t('editor.save'),
      confirmLabel: t('editor.discard'),
      cancelLabel: t('editor.keepEditing'),
      confirmColor: 'error',
    })
    if (result === 'cancel')
      return
    if (result === 'save') {
      await editorStore.saveTab(tab.id)
      if (tab.error) // save failed — keep the tab open so the work isn't lost
        return
    }
  }
  editorStore.closeTab(tab.id)
}
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
      @mousedown.middle.prevent="requestClose(tab)"
    >
      <span class="max-w-24 sm:max-w-32 truncate">{{ tab.name }}</span>
      <span
        v-if="tab.content !== tab.savedContent"
        class="size-2 rounded-full bg-warning shrink-0"
        :title="t('editor.unsavedChanges')"
        :aria-label="t('editor.unsavedChanges')"
      />
      <UButton
        size="xs"
        color="neutral"
        variant="ghost"
        icon="i-lucide-x"
        class="-mr-1"
        :aria-label="t('editor.closeTab')"
        @click.stop="requestClose(tab)"
      />
    </div>

    <div class="flex-1 min-w-4" />

    <div v-if="!viewOnly" class="px-3 flex items-center gap-3 shrink-0">
      <USwitch
        v-model="settingsStore.editorAutoSave"
        size="xs"
        :label="t('editor.autoSave')"
        :ui="{ label: 'text-xs font-mono text-muted whitespace-nowrap' }"
      />

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
