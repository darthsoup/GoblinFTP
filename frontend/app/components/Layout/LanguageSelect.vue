<script setup lang="ts">
import type { AppLanguage } from '~/stores/settings'
import { de, en } from '@nuxt/ui/locale'

const { locale, setLocale, t } = useI18n()
const settingsStore = useSettingsStore()

// Nuxt UI's locale objects carry the code + display name (+ flag) ULocaleSelect
// renders; we only switch the app's own en/de.
const locales = [en, de]

// Explicit user choice persists to the settings store so it overrides the
// admin default (mirrors the language logic in SettingsModal).
const current = computed<string>({
  get: () => locale.value,
  set: (v) => {
    settingsStore.language = v as AppLanguage
    setLocale(v as typeof locale.value)
  },
})
</script>

<template>
  <ULocaleSelect
    v-model="current"
    :locales="locales"
    :search-input="false"
    :aria-label="t('settings.language')"
  />
</template>
