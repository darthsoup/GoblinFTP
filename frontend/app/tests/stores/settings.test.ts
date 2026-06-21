import type { SystemVars } from '~/types/api'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

function systemVars(showDotFiles: boolean): SystemVars {
  return {
    language: 'en',
    ui: { pageTitle: 'GoblinFTP', showDotFiles, showNavigationHistory: true },
    branding: { appName: 'GoblinFTP', logoUrl: null, faviconUrl: null, primaryColor: null, tagline: null, hideAttribution: false },
    upload: { chunkSize: 1, maxConcurrentUploads: 1 },
    connection: { allowedTypes: ['ftp'], disableChmod: false, presetHost: null, presetPort: null, lockHost: false, passiveMode: true },
    editor: { disabled: false, viewOnly: false, allowedExtensions: [] },
    loginFormDisabled: false,
    ssoEnabled: false,
    frontendLogEnabled: false,
    version: 'dev',
  }
}

describe('useSettingsStore', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  it('follows the admin default until the user chooses', () => {
    const auth = useAuthStore()
    const store = useSettingsStore()
    expect(store.showDotfiles).toBe(false) // backend default

    auth.systemVars = systemVars(true)
    expect(store.showDotfiles).toBe(true) // admin default applies reactively
  })

  it('user choice overrides the admin default and persists', async () => {
    const auth = useAuthStore()
    auth.systemVars = systemVars(true)
    const store = useSettingsStore()

    store.showDotfiles = false
    await nextTick()
    expect(store.showDotfiles).toBe(false)
    expect(JSON.parse(localStorage.getItem('gftp_settings')!).showDotfiles).toBe(false)
  })

  it('restores persisted preferences on a fresh store', () => {
    localStorage.setItem('gftp_settings', JSON.stringify({
      showDotfiles: true,
      language: 'de',
      editorAutoSave: true,
      sizeFormat: 'bytes',
      dateFormat: 'relative',
    }))
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.showDotfiles).toBe(true)
    expect(store.language).toBe('de')
    expect(store.editorAutoSave).toBe(true)
    expect(store.sizeFormat).toBe('bytes')
    expect(store.dateFormat).toBe('relative')
  })

  it('persists the file view mode and restores it', async () => {
    localStorage.setItem('gftp_settings', JSON.stringify({ fileViewMode: 'cards' }))
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.fileViewMode).toBe('cards')

    store.fileViewMode = 'table'
    await nextTick()
    expect(JSON.parse(localStorage.getItem('gftp_settings')!).fileViewMode).toBe('table')
  })

  it('persists the grid-thumbnails preference (default on)', async () => {
    setActivePinia(createPinia())
    expect(useSettingsStore().gridThumbnails).toBe(true)

    localStorage.setItem('gftp_settings', JSON.stringify({ gridThumbnails: false }))
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.gridThumbnails).toBe(false)

    store.gridThumbnails = true
    await nextTick()
    expect(JSON.parse(localStorage.getItem('gftp_settings')!).gridThumbnails).toBe(true)
  })

  it('ignores corrupt or invalid persisted data', () => {
    localStorage.setItem('gftp_settings', JSON.stringify({ sizeFormat: 'bogus', language: 'xx' }))
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.sizeFormat).toBe('binary')
    expect(store.language).toBeNull()

    localStorage.setItem('gftp_settings', 'not-json{')
    setActivePinia(createPinia())
    expect(useSettingsStore().showDotfiles).toBe(false)
  })
})
