<template>
  <UModal v-model:open="open" :title="t('settings.title')">
    <template #title>
      <UIcon name="i-lucide-settings" class="size-5 text-muted" />
      {{ t('settings.title') }}
    </template>

    <template #body>
      <div class="space-y-5">
        <UFormField :label="t('settings.language')">
          <LanguageSelect class="w-full" />
        </UFormField>

        <UFormField :label="t('settings.theme')">
          <USelect
            v-model="theme"
            :items="themeItems"
            class="w-full"
          />
        </UFormField>

        <div class="grid grid-cols-2 gap-3">
          <UFormField :label="t('settings.sizeFormat')">
            <USelect
              v-model="settingsStore.sizeFormat"
              :items="sizeFormatItems"
              class="w-full"
            />
          </UFormField>
          <UFormField :label="t('settings.dateFormat')">
            <USelect
              v-model="settingsStore.dateFormat"
              :items="dateFormatItems"
              class="w-full"
            />
          </UFormField>
        </div>

        <UFormField :label="t('settings.density')">
          <USelect
            v-model="settingsStore.density"
            :items="densityItems"
            class="w-full"
          />
        </UFormField>

        <UFormField :label="t('settings.showDotfiles')" :description="t('settings.showDotfilesHint')">
          <USwitch v-model="settingsStore.showDotfiles" />
        </UFormField>

        <UFormField :label="t('settings.gridThumbnails')" :description="t('settings.gridThumbnailsHint')">
          <USwitch v-model="settingsStore.gridThumbnails" />
        </UFormField>
      </div>
    </template>

    <template #footer="{ close }">
      <div class="flex w-full items-center justify-between">
        <UButton :label="t('settings.close')" @click="close" />
        <!-- Brand + semver is locale-invariant, so no i18n key is needed. -->
        <span
          v-if="!hideAttribution && !embedded"
          class="text-xs text-dimmed"
        >
          {{ appName }} {{ authStore.systemVars?.version ?? '' }}
        </span>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const authStore = useAuthStore()
const colorMode = useColorMode()
const { apply: applyColorMode } = useColorModeTransition()
const { t } = useI18n()
const { appName, hideAttribution } = useBranding()
const { embedded } = useEmbed()

const open = computed({
  get: () => modalStore.active === 'settings',
  set: (v: boolean) => {
    if (!v)
      modalStore.close()
  },
})

// Changes apply immediately, no save/cancel dance. Language lives in
// <LanguageSelect>; the rest is remembered in the settings store.
const theme = computed({
  get: () => colorMode.preference,
  set: (v: string) => {
    applyColorMode(v) // cross-fade the flip (no pointer origin here)
  },
})
const themeItems = computed(() => [
  { label: t('settings.themeLight'), value: 'light' },
  { label: t('settings.themeDark'), value: 'dark' },
  { label: t('settings.themeAuto'), value: 'system' },
])

const sizeFormatItems = computed(() => [
  { label: t('settings.sizeBinary'), value: 'binary' },
  { label: t('settings.sizeDecimal'), value: 'decimal' },
  { label: t('settings.sizeBytes'), value: 'bytes' },
])
const dateFormatItems = computed(() => [
  { label: t('settings.dateAuto'), value: 'auto' },
  { label: t('settings.dateAbsolute'), value: 'absolute' },
  { label: t('settings.dateRelative'), value: 'relative' },
])
const densityItems = computed(() => [
  { label: t('settings.densityComfortable'), value: 'comfortable' },
  { label: t('settings.densityCompact'), value: 'compact' },
])
</script>
