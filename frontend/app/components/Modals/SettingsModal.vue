<script setup lang="ts">
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const colorMode = useColorMode()
const { t, locale, setLocale } = useI18n()

const open = computed({
  get: () => modalStore.active === 'settings',
  set: (v: boolean) => {
    if (!v)
      modalStore.close()
  },
})

// Changes apply immediately — no save/cancel dance.
const language = computed({
  get: () => locale.value,
  set: (v: typeof locale.value) => {
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

        <UFormField :label="t('settings.theme')" :description="t('settings.themeHint')">
          <USelect
            v-model="theme"
            :items="themeItems"
            class="w-full font-mono"
          />
        </UFormField>

        <UFormField :label="t('settings.showDotfiles')" :description="t('settings.showDotfilesHint')">
          <USwitch v-model="settingsStore.showDotfiles" />
        </UFormField>
      </div>
    </template>

    <template #footer="{ close }">
      <UButton :label="t('settings.close')" @click="close" />
    </template>
  </UModal>
</template>
