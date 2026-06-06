import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

describe('useSettingsStore', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
  })

  it('defaults to showing dotfiles', () => {
    const store = useSettingsStore()
    expect(store.showDotfiles).toBe(true)
  })

  it('persists changes to localStorage', async () => {
    const store = useSettingsStore()
    store.showDotfiles = false
    await nextTick()
    expect(JSON.parse(localStorage.getItem('gftp_settings')!)).toEqual({ showDotfiles: false })
  })

  it('restores the persisted value on a fresh store', () => {
    localStorage.setItem('gftp_settings', JSON.stringify({ showDotfiles: false }))
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    const store = useSettingsStore()
    expect(store.showDotfiles).toBe(false)
  })

  it('ignores corrupt persisted data', () => {
    localStorage.setItem('gftp_settings', 'not-json{')
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    const store = useSettingsStore()
    expect(store.showDotfiles).toBe(true)
  })
})
