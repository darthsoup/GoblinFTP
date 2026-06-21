<script setup lang="ts">
import type { AppLanguage } from '~/stores/settings'
import { cs, da, de, en, es, fi, fr, it, nb_no, nl, pt, sk, sv } from '@nuxt/ui/locale'

const { locale, setLocale, t } = useI18n()
const settingsStore = useSettingsStore()

// @nuxt/ui locale objects give ULocaleSelect its native names; Danish ships as "Danish", overridden to its endonym.
const locales = [
  en,
  de,
  cs,
  { ...da, name: 'Dansk' },
  es,
  fi,
  fr,
  it,
  nl,
  nb_no,
  pt,
  sk,
  sv,
].sort((a, b) => a.name.localeCompare(b.name))

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
