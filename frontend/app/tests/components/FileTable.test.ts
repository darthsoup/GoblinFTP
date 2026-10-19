import type { VueWrapper } from '@vue/test-utils'
import type { FileInfo } from '~/types/api'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import { createTestingPinia } from '@pinia/testing'
import { DOMWrapper, flushPromises } from '@vue/test-utils'
import { setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import FileTable from '~/components/FileBrowser/FileTable.vue'

let wrapper: VueWrapper | undefined
let pinia: ReturnType<typeof createTestingPinia>

const FileRowStub = defineComponent({
  props: ['file'],
  template: '<tr :data-file-name="file.name"><td>{{ file.name }}</td></tr>',
})

async function mountTable() {
  wrapper = await mountSuspended(FileTable, {
    attachTo: document.body,
    global: {
      plugins: [pinia],
      stubs: {
        FileRow: FileRowStub,
        FileToolbar: true,
        FilePreviewPanel: true,
        UContextMenu: { props: ['items'], template: '<slot />' },
      },
    },
  })
  return wrapper
}

function names() {
  return wrapper!.findAll('tbody [data-file-name]').map(row => row.attributes('data-file-name'))
}

async function sortWithMenu(index: number) {
  await wrapper!.get('button[aria-label="Sort by"]').trigger('click', { button: 0, ctrlKey: false })
  await flushPromises()
  const item = document.querySelectorAll<HTMLElement>('[role="menuitemcheckbox"]')[index]
  expect(item).toBeDefined()
  await new DOMWrapper(item!).trigger('click')
  await flushPromises()
}

describe('file table sorting', () => {
  beforeEach(() => {
    localStorage.clear()
    pinia = createTestingPinia({ createSpy: vi.fn, stubActions: false })
    setActivePinia(pinia)
    useSettingsStore().fileViewMode = 'table'
    const files: FileInfo[] = [
      { name: 'zeta.txt', size: 10, isDir: false, modified: '2026-01-01T00:00:00Z', mode: '' },
      { name: 'alpha.txt', size: 100, isDir: false, modified: '2026-01-03T00:00:00Z', mode: '' },
      { name: 'docs', size: 0, isDir: true, modified: '2026-01-01T00:00:00Z', mode: '' },
      { name: 'middle.txt', size: 50, isDir: false, modified: '2026-01-02T00:00:00Z', mode: '' },
    ]
    useFilesStore().files = files
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
  })

  it('sorts by size in either direction and restores server order through the Name menu', async () => {
    const table = await mountTable()
    expect(names()).toEqual(['docs', 'alpha.txt', 'middle.txt', 'zeta.txt'])
    expect(table.findAll('thead th')).toHaveLength(4)

    await sortWithMenu(2)
    expect(names()).toEqual(['docs', 'zeta.txt', 'middle.txt', 'alpha.txt'])
    expect(table.get('th[aria-sort="ascending"]').text()).toContain('Name')
    expect(table.get('[aria-live="polite"]').text()).toBe('Sorted by Size: Ascending')

    await sortWithMenu(3)
    expect(names()).toEqual(['docs', 'alpha.txt', 'middle.txt', 'zeta.txt'])
    expect(table.get('[aria-live="polite"]').text()).toBe('Sorted by Size: Descending')

    await sortWithMenu(4)
    expect(names()).toEqual(['zeta.txt', 'alpha.txt', 'docs', 'middle.txt'])
    expect(table.get('[aria-live="polite"]').text()).toBe('Server order')
  })

  it('offers both Name directions and preserves Modified header sorting', async () => {
    const table = await mountTable()
    await sortWithMenu(1)
    expect(names()).toEqual(['docs', 'zeta.txt', 'middle.txt', 'alpha.txt'])
    expect(table.get('th[aria-sort="descending"]').text()).toContain('Name')

    await sortWithMenu(0)
    expect(names()).toEqual(['docs', 'alpha.txt', 'middle.txt', 'zeta.txt'])

    const modified = table.findAll('thead button').find(button => button.text() === 'Modified')!
    await modified.trigger('click')
    expect(names()).toEqual(['docs', 'zeta.txt', 'middle.txt', 'alpha.txt'])
    await modified.trigger('click')
    expect(names()).toEqual(['docs', 'alpha.txt', 'middle.txt', 'zeta.txt'])
    await modified.trigger('click')
    expect(names()).toEqual(['zeta.txt', 'alpha.txt', 'docs', 'middle.txt'])
  })

  it('dismisses the sort menu on Escape while file browser shortcuts are active', async () => {
    const table = await mountTable()
    const trigger = table.get('button[aria-label="Sort by"]')
    await trigger.trigger('click', { button: 0, ctrlKey: false })
    await flushPromises()
    expect(trigger.attributes('aria-expanded')).toBe('true')

    const menu = document.querySelector<HTMLElement>('[role="menu"]')!
    await new DOMWrapper(menu).trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })

  it('dismisses a card action menu on Escape while file browser shortcuts are active', async () => {
    useSettingsStore(pinia).fileViewMode = 'cards'
    const table = await mountTable()
    const trigger = table.get('[data-file-name="zeta.txt"] button[aria-label="Actions"]')
    await trigger.trigger('click', { button: 0, ctrlKey: false })
    await flushPromises()
    expect(trigger.attributes('aria-expanded')).toBe('true')

    const menu = document.querySelector<HTMLElement>('[role="menu"]')!
    await new DOMWrapper(menu).trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })

  it('sorts completed folder totals and keeps partial or unknown folders last in either direction', async () => {
    const files = useFilesStore(pinia)
    files.currentPath = '/pub'
    files.files = ['unknown', 'partial', 'large', 'empty', 'equal-b', 'equal-a'].map(name => ({
      name,
      size: 999,
      isDir: true,
      modified: '2026-01-01T00:00:00Z',
      mode: '',
    }))
    files.files.push({ name: 'file.txt', size: 1, isDir: false, modified: '2026-01-01T00:00:00Z', mode: '' })
    const sizes = useDirectorySizesStore(pinia)
    for (const [name, bytes] of [['large', 100], ['empty', 0], ['equal-b', 10], ['equal-a', 10], ['partial', 200]] as const) {
      const complete = name !== 'partial'
      sizes.entries[`/pub/${name}`] = {
        status: complete ? 'complete' : 'partial',
        result: { path: `/pub/${name}`, bytes, hasEntries: true, complete, fileCount: 1, directoryCount: 0, skippedCount: 0, calculatedAt: '2026-10-19T12:00:00Z' },
      }
    }
    await mountTable()
    await sortWithMenu(2)
    expect(names()).toEqual(['empty', 'equal-a', 'equal-b', 'large', 'partial', 'unknown', 'file.txt'])
    await sortWithMenu(3)
    expect(names()).toEqual(['large', 'equal-a', 'equal-b', 'empty', 'partial', 'unknown', 'file.txt'])
    sizes.entries['/pub/large']!.status = 'loading'
    await nextTick()
    expect(names()).toEqual(['large', 'equal-a', 'equal-b', 'empty', 'partial', 'unknown', 'file.txt'])
    sizes.activePath = '/pub/large'
    sizes.cancel()
    await nextTick()
    expect(names()).toEqual(['large', 'equal-a', 'equal-b', 'empty', 'partial', 'unknown', 'file.txt'])
    sizes.entries['/pub/large']!.status = 'error'
    await nextTick()
    expect(names()).toEqual(['large', 'equal-a', 'equal-b', 'empty', 'partial', 'unknown', 'file.txt'])
    expect(files.files.every(file => !file.isDir || file.size === 999)).toBe(true)
  })

  it('opens folder Properties from the shared card menu without starting a calculation', async () => {
    useSettingsStore(pinia).fileViewMode = 'cards'
    const sizes = useDirectorySizesStore(pinia)
    const calculate = vi.spyOn(sizes, 'calculate').mockResolvedValue()
    const table = await mountTable()
    await table.get('[data-file-name="docs"] button[aria-label="Actions"]').trigger('click', { button: 0, ctrlKey: false })
    await flushPromises()
    const actions = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]'))
    expect(actions.some(item => item.textContent?.includes('size'))).toBe(false)
    const action = actions.find(item => item.textContent === 'Properties')!
    await new DOMWrapper(action).trigger('click')
    await flushPromises()
    expect(useModalStore(pinia).active).toBe('properties')
    expect(useModalStore(pinia).context.file?.name).toBe('docs')
    expect(calculate).not.toHaveBeenCalled()
    expect(useFilesStore(pinia).currentPath).toBe('/')
  })
})
