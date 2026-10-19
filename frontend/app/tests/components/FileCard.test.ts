import type { DropdownMenuItem } from '@nuxt/ui'
import type { VueWrapper } from '@vue/test-utils'
import type { FileInfo } from '~/types/api'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import { createTestingPinia } from '@pinia/testing'
import { DOMWrapper, flushPromises } from '@vue/test-utils'
import { setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FileCard from '~/components/FileBrowser/FileCard.vue'

const wrappers: VueWrapper[] = []
let pinia: ReturnType<typeof createTestingPinia>

async function mountCard(over: Partial<FileInfo> = {}, menuItems: DropdownMenuItem[][] = []) {
  const wrapper = await mountSuspended(FileCard, {
    attachTo: document.body,
    global: { plugins: [pinia] },
    props: {
      file: { name: 'report.pdf', size: 2048, isDir: false, modified: '2026-01-01T00:00:00Z', mode: '-rw-r--r--', ...over },
      selected: false,
      currentPath: '/pub',
      editing: false,
      isCut: false,
      active: false,
      compact: false,
      index: 0,
      menuItems,
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('file cards', () => {
  beforeEach(() => {
    localStorage.clear()
    pinia = createTestingPinia({ createSpy: vi.fn, stubActions: false })
    setActivePinia(pinia)
    useSettingsStore().sizeFormat = 'bytes'
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  })

  it('shows the file size without a modified date and leaves folder metadata empty', async () => {
    const file = await mountCard()
    expect(file.text()).toBe('report.pdf2,048 B')

    const folder = await mountCard({ name: 'docs', isDir: true })
    expect(folder.text()).toBe('docs')
  })

  it.each([
    { name: 'report.pdf', isDir: false },
    { name: 'docs', isDir: true },
  ])('runs a menu action on $name without opening the card', async (file) => {
    const action = vi.fn()
    const wrapper = await mountCard(file, [[{ label: 'Test action', onSelect: action }]])
    await wrapper.get('button[aria-label="Actions"]').trigger('click', { button: 0, ctrlKey: false })
    await flushPromises()

    const item = document.querySelector<HTMLElement>('[role="menuitem"]')
    expect(item).not.toBeNull()
    await new DOMWrapper(item!).trigger('click')
    await flushPromises()

    expect(action).toHaveBeenCalledOnce()
    expect(wrapper.emitted('preview')).toBeUndefined()
    expect(wrapper.emitted('navigate')).toBeUndefined()
  })

  it('keeps menu and checkbox keyboard events separate from card navigation', async () => {
    const wrapper = await mountCard({}, [[{ label: 'Test action' }]])
    const menu = wrapper.get('button[aria-label="Actions"]')
    await menu.trigger('keydown', { key: 'ArrowDown' })
    await menu.trigger('keydown', { key: 'Enter' })
    const checkbox = wrapper.get('[role="checkbox"]')
    await checkbox.trigger('keydown', { key: ' ' })

    expect(wrapper.emitted('preview')).toBeUndefined()
    expect(wrapper.emitted('navigate')).toBeUndefined()
    expect(wrapper.emitted('focusMove')).toBeUndefined()
    expect(wrapper.emitted('select')).toBeUndefined()
    await checkbox.trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
  })

  it('still opens a focused card on Enter', async () => {
    const wrapper = await mountCard({ name: 'docs', isDir: true })
    await wrapper.get('[data-file-card]').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('navigate')).toEqual([['/pub/docs']])
  })
})
