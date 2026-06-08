<script setup lang="ts">
const authStore = useAuthStore()
const filesStore = useFilesStore()
const editorStore = useEditorStore()
const modalStore = useModalStore()
const route = useRoute()
const { t } = useI18n()

async function handleDisconnect() {
  if (editorStore.hasDirty) {
    const result = await modalStore.confirm({
      title: t('editor.unsavedTitle'),
      message: t('editor.confirmDisconnectMessage', { n: editorStore.dirtyCount }),
      confirmLabel: t('header.disconnect'),
      cancelLabel: t('editor.keepEditing'),
      confirmColor: 'error',
    })
    if (result !== 'confirm')
      return
  }
  await authStore.disconnect()
  filesStore.$reset()
  editorStore.$reset()
}
</script>

<template>
  <UHeader
    title="GoblinFTP"
    :toggle="false"
    :ui="{
      root: 'bg-muted/75 shrink-0 z-30',
      container: 'max-w-full px-4 sm:px-4 lg:px-4 gap-2',
    }"
  >
    <template #left>
      <div class="flex items-center gap-2 select-none">
        <UIcon name="i-lucide-server" class="size-5 text-primary" />
        <span class="text-xl font-bold tracking-tight text-primary">GoblinFTP</span>
      </div>
    </template>

    <template #right>
      <UButton
        v-if="editorStore.hasOpenTabs && route.path !== '/edit'"
        color="neutral"
        variant="ghost"
        icon="i-lucide-file-pen"
        :label="t('editor.openEditor', { n: editorStore.tabs.length })"
        @click="navigateTo('/edit')"
      />

      <UColorModeButton />

      <UTooltip :text="t('header.settings')">
        <UButton
          color="neutral"
          variant="ghost"
          icon="i-lucide-settings"
          :aria-label="t('header.settings')"
          @click="modalStore.open('settings')"
        />
      </UTooltip>

      <USeparator orientation="vertical" class="h-5 mx-1" />

      <UButton
        color="error"
        variant="ghost"
        icon="i-lucide-log-out"
        :label="t('header.disconnect')"
        @click="handleDisconnect"
      />
    </template>
  </UHeader>
</template>
