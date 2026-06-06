import { defineStore } from 'pinia'

// End-user UI preferences. Persisted in the browser only (localStorage) —
// the backend never needs them: dotfile filtering, language, and theme are
// all client-side concerns. Language is persisted by @nuxtjs/i18n (cookie)
// and theme by @nuxtjs/color-mode (localStorage); this store covers the rest.
const STORAGE_KEY = 'gftp_settings'

interface PersistedSettings {
  showDotfiles?: boolean
}

export const useSettingsStore = defineStore('settings', () => {
  const showDotfiles = ref(true)

  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as PersistedSettings
      if (typeof parsed.showDotfiles === 'boolean')
        showDotfiles.value = parsed.showDotfiles
    }
  }
  catch {}

  watch(showDotfiles, () => {
    try {
      const persisted: PersistedSettings = { showDotfiles: showDotfiles.value }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(persisted))
    }
    catch {}
  })

  return { showDotfiles }
})
