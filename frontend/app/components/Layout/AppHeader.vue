<script setup lang="ts">
import type { NavigationMenuItem } from '@nuxt/ui'

const authStore = useAuthStore()
const filesStore = useFilesStore()
const editorStore = useEditorStore()
const modalStore = useModalStore()
const route = useRoute()
const { t } = useI18n()
const { appName, logoUrl } = useBranding()

// Centre switcher between the file browser and the editor — only relevant while
// the editor has open tabs. The Files link carries the current browse path so
// returning from the editor reopens the same folder.
const navItems = computed<NavigationMenuItem[]>(() => [
  {
    label: t('header.files'),
    icon: 'i-lucide-folder',
    to: { path: '/', query: { path: filesStore.currentPath } },
    active: route.path === '/',
  },
  {
    label: t('header.editor'),
    icon: 'i-lucide-file-pen',
    to: '/edit',
    active: route.path === '/edit',
    badge: editorStore.tabs.length,
  },
])

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
    :title="appName"
    :toggle="false"
    :ui="{
      root: 'bg-muted/75 shrink-0 z-30',
      container: 'max-w-full px-3 sm:px-4 gap-2',
      center: 'flex',
    }"
  >
    <template #left>
      <div class="flex items-center gap-2 select-none">
        <img v-if="logoUrl" :src="logoUrl" :alt="appName" class="size-6 object-contain">
        <UIcon v-else name="i-lucide-server" class="size-5 text-primary" />
        <span class="text-lg sm:text-xl font-bold tracking-tight text-primary truncate max-w-[40vw] sm:max-w-none">{{ appName }}</span>
      </div>
    </template>

    <UNavigationMenu
      v-if="editorStore.hasOpenTabs"
      :items="navItems"
      variant="pill"
      color="primary"
    />

    <template #right>
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
        :aria-label="t('header.disconnect')"
        @click="handleDisconnect"
      >
        <span class="hidden sm:inline">{{ t('header.disconnect') }}</span>
      </UButton>
    </template>
  </UHeader>
</template>
