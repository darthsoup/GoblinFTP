<script setup lang="ts">
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const authStore = useAuthStore()
const colorMode = useColorMode()
const { t, locale, setLocale } = useI18n()

const open = computed({
  get: () => modalStore.active === 'settings',
  set: (v: boolean) => {
    if (!v)
      modalStore.close()
  },
})

// Changes apply immediately — no save/cancel dance. The explicit choice is
// remembered in the settings store so it overrides the admin default.
const language = computed({
  get: () => locale.value,
  set: (v: typeof locale.value) => {
    settingsStore.language = v
    setLocale(v)
  },
})
const languageItems = [
  { label: 'English', value: 'en' },
  { label: 'Deutsch', value: 'de' },
] as const

const theme = computed({
  get: () => colorMode.preference,
  set: (v: string) => {
    colorMode.preference = v
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
</script>

<template>
  <UModal v-model:open="open" :title="t('settings.title')">
    <template #title>
      <UIcon name="i-lucide-settings" class="size-5 text-muted" />
      {{ t('settings.title') }}
    </template>

    <template #body>
      <div class="space-y-5">
        <UFormField :label="t('settings.language')">
          <USelect
            v-model="language"
            :items="[...languageItems]"
            class="w-full font-mono"
          />
        </UFormField>

        <UFormField :label="t('settings.theme')">
          <USelect
            v-model="theme"
            :items="themeItems"
            class="w-full font-mono"
          />
        </UFormField>

        <div class="grid grid-cols-2 gap-3">
          <UFormField :label="t('settings.sizeFormat')">
            <USelect
              v-model="settingsStore.sizeFormat"
              :items="sizeFormatItems"
              class="w-full font-mono"
            />
          </UFormField>
          <UFormField :label="t('settings.dateFormat')">
            <USelect
              v-model="settingsStore.dateFormat"
              :items="dateFormatItems"
              class="w-full font-mono"
            />
          </UFormField>
        </div>

        <UFormField :label="t('settings.showDotfiles')" :description="t('settings.showDotfilesHint')">
          <USwitch v-model="settingsStore.showDotfiles" />
        </UFormField>
      </div>
    </template>

    <template #footer="{ close }">
      <div class="flex w-full items-center justify-between">
        <!-- Brand + semver is locale-invariant — no i18n key needed. -->
        <span class="font-mono text-xs text-dimmed">GoblinFTP {{ authStore.systemVars?.version ?? '' }}</span>
        <UButton :label="t('settings.close')" @click="close" />
      </div>
    </template>
  </UModal>
</template>
