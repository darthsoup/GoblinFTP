import { defineStore } from 'pinia'

// End-user UI preferences. Persisted in the browser only (localStorage) —
// the backend never needs them: dotfile filtering, language, theme, and
// formatting are all client-side concerns. Theme is persisted by
// @nuxtjs/color-mode (localStorage); this store covers the rest.
//
// Preferences with an admin-level default in settings.json (dotfiles,
// language) follow "user override wins, otherwise admin default": the user
// value stays null until the user explicitly changes the setting.
const STORAGE_KEY = 'gftp_settings'

export type SizeFormat = 'binary' | 'decimal' | 'bytes'
export type DateFormat = 'auto' | 'absolute' | 'relative'
export type AppLanguage = 'en' | 'de' | 'cs' | 'da' | 'es' | 'fi' | 'fr' | 'it' | 'nb-NO' | 'nl' | 'pt' | 'sk' | 'sv'
export type FileViewMode = 'table' | 'cards'

interface PersistedSettings {
  showDotfiles?: boolean | null
  language?: AppLanguage | null
  editorAutoSave?: boolean
  sizeFormat?: SizeFormat
  dateFormat?: DateFormat
  fileViewMode?: FileViewMode
  gridThumbnails?: boolean
}

const SIZE_FORMATS: SizeFormat[] = ['binary', 'decimal', 'bytes']
const DATE_FORMATS: DateFormat[] = ['auto', 'absolute', 'relative']
export const LANGUAGES: AppLanguage[] = ['en', 'de', 'cs', 'da', 'es', 'fi', 'fr', 'it', 'nb-NO', 'nl', 'pt', 'sk', 'sv']

// Narrow the admin default (settings.json) to a supported language; shared with the auth plugin.
export function isAppLanguage(value: string | null | undefined): value is AppLanguage {
  return value != null && (LANGUAGES as readonly string[]).includes(value)
}
const FILE_VIEW_MODES: FileViewMode[] = ['table', 'cards']

// Until the user picks, default to cards on phone-width viewports, table above.
function defaultFileViewMode(): FileViewMode {
  return typeof window !== 'undefined' && window.innerWidth < 640 ? 'cards' : 'table'
}

export const useSettingsStore = defineStore('settings', () => {
  const authStore = useAuthStore()

  // null = no explicit user choice → follow the admin default from systemVars
  const userShowDotfiles = ref<boolean | null>(null)
  const language = ref<AppLanguage | null>(null)
  const editorAutoSave = ref(false)
  const sizeFormat = ref<SizeFormat>('binary')
  const dateFormat = ref<DateFormat>('auto')
  const fileViewMode = ref<FileViewMode>(defaultFileViewMode())
  // Show image thumbnails in the card grid (end-user preference).
  const gridThumbnails = ref(true)

  const showDotfiles = computed({
    get: () => userShowDotfiles.value ?? authStore.systemVars?.ui.showDotFiles ?? false,
    set: (v: boolean) => {
      userShowDotfiles.value = v
    },
  })

  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as PersistedSettings
      if (typeof parsed.showDotfiles === 'boolean')
        userShowDotfiles.value = parsed.showDotfiles
      if (parsed.language && LANGUAGES.includes(parsed.language))
        language.value = parsed.language
      if (typeof parsed.editorAutoSave === 'boolean')
        editorAutoSave.value = parsed.editorAutoSave
      if (parsed.sizeFormat && SIZE_FORMATS.includes(parsed.sizeFormat))
        sizeFormat.value = parsed.sizeFormat
      if (parsed.dateFormat && DATE_FORMATS.includes(parsed.dateFormat))
        dateFormat.value = parsed.dateFormat
      if (parsed.fileViewMode && FILE_VIEW_MODES.includes(parsed.fileViewMode))
        fileViewMode.value = parsed.fileViewMode
      if (typeof parsed.gridThumbnails === 'boolean')
        gridThumbnails.value = parsed.gridThumbnails
    }
  }
  catch {}

  watch([userShowDotfiles, language, editorAutoSave, sizeFormat, dateFormat, fileViewMode, gridThumbnails], () => {
    try {
      const persisted: PersistedSettings = {
        showDotfiles: userShowDotfiles.value,
        language: language.value,
        editorAutoSave: editorAutoSave.value,
        sizeFormat: sizeFormat.value,
        dateFormat: dateFormat.value,
        fileViewMode: fileViewMode.value,
        gridThumbnails: gridThumbnails.value,
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(persisted))
    }
    catch {}
  })

  return { showDotfiles, language, editorAutoSave, sizeFormat, dateFormat, fileViewMode, gridThumbnails }
})
