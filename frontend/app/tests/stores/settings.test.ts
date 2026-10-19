import type { SystemVars } from '~/types/api'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

function systemVars(showDotFiles: boolean): SystemVars {
  return {
    language: 'en',
    ui: { pageTitle: 'GoblinFTP', showDotFiles, showNavigationHistory: true },
    branding: { appName: 'GoblinFTP', logoUrl: null, logoDarkUrl: null, faviconUrl: null, primaryColor: null, primaryTextColor: null, hideAttribution: false, themeCssUrl: null },
    upload: { chunkSize: 1, maxConcurrentUploads: 1 },
    connection: { allowedTypes: ['ftp'], disableChmod: false, presetHost: null, presetPort: null, lockHost: false, passiveMode: true },
    editor: { disabled: false, viewOnly: false, allowedExtensions: [] },
    embed: { chromeless: 'auto' },
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

  it('persists the grid-thumbnails preference (default off)', async () => {
    setActivePinia(createPinia())
    expect(useSettingsStore().gridThumbnails).toBe(false)

    localStorage.setItem('gftp_settings', JSON.stringify({ gridThumbnails: true }))
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.gridThumbnails).toBe(true)

    store.gridThumbnails = false
    await nextTick()
    expect(JSON.parse(localStorage.getItem('gftp_settings')!).gridThumbnails).toBe(false)
  })

  it('defaults to standard density', () => {
    expect(useSettingsStore().density).toBe('standard')
  })

  it('restores legacy comfortable density as standard without losing other preferences', async () => {
    localStorage.setItem('gftp_settings', JSON.stringify({ density: 'comfortable', sizeFormat: 'bytes' }))
    const store = useSettingsStore()
    expect(store.density).toBe('standard')
    expect(store.sizeFormat).toBe('bytes')

    store.gridThumbnails = true
    await nextTick()
    expect(JSON.parse(localStorage.getItem('gftp_settings')!).density).toBe('standard')
  })

  it.each(['standard', 'compact'] as const)('restores %s density', (density) => {
    localStorage.setItem('gftp_settings', JSON.stringify({ density }))
    expect(useSettingsStore().density).toBe(density)
  })

  it('persists both density options', async () => {
    const store = useSettingsStore()
    store.density = 'compact'
    await nextTick()
    expect(JSON.parse(localStorage.getItem('gftp_settings')!).density).toBe('compact')

    store.density = 'standard'
    await nextTick()
    expect(JSON.parse(localStorage.getItem('gftp_settings')!).density).toBe('standard')
  })

  it('ignores corrupt or invalid persisted data', () => {
    localStorage.setItem('gftp_settings', JSON.stringify({ sizeFormat: 'bogus', language: 'xx', density: 'bogus' }))
    setActivePinia(createPinia())
    const store = useSettingsStore()
    expect(store.sizeFormat).toBe('binary')
    expect(store.language).toBeNull()
    expect(store.density).toBe('standard')

    localStorage.setItem('gftp_settings', 'not-json{')
    setActivePinia(createPinia())
    expect(useSettingsStore().showDotfiles).toBe(false)
  })
})
