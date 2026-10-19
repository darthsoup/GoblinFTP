import type { VueWrapper } from '@vue/test-utils'
import type { DirectorySizeResult } from '~/types/api'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DirectorySize from '~/components/FileBrowser/DirectorySize.vue'
import FileCard from '~/components/FileBrowser/FileCard.vue'
import FileRow from '~/components/FileBrowser/FileRow.vue'
import PropertiesModal from '~/components/Modals/PropertiesModal.vue'

const result: DirectorySizeResult = { path: '/pub/docs', bytes: 2048, hasEntries: true, fileCount: 3, directoryCount: 2, complete: true, skippedCount: 0, calculatedAt: '2026-10-19T12:00:00Z' }
const wrappers: VueWrapper[] = []
let pinia: ReturnType<typeof createTestingPinia>

async function mountSize(details = false) {
  const wrapper = await mountSuspended(DirectorySize, { props: { path: '/pub/docs', details }, global: { plugins: [pinia] } })
  wrappers.push(wrapper)
  return wrapper
}

describe('shared directory size display', () => {
  beforeEach(() => {
    localStorage.clear()
    pinia = createTestingPinia({ createSpy: vi.fn, stubActions: false })
    setActivePinia(pinia)
    useSettingsStore().sizeFormat = 'bytes'
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  })

  it('leaves unknown sizes blank and offers calculation in Properties', async () => {
    const wrapper = await mountSize(true)
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.get('button').text()).toBe('Calculate size')
  })

  it('shows loading and cancellation, then labels an empty directory', async () => {
    const sizes = useDirectorySizesStore()
    sizes.entries['/pub/docs'] = { status: 'loading' }
    const cancel = vi.spyOn(sizes, 'cancel')
    const wrapper = await mountSize(true)
    expect(wrapper.get('[role="status"]').text()).toBe('Calculating...')
    await wrapper.get('button').trigger('click')
    expect(cancel).toHaveBeenCalledOnce()
    sizes.entries['/pub/docs'] = { status: 'complete', result: { ...result, bytes: 0, hasEntries: false, fileCount: 0, directoryCount: 0 } }
    await nextTick()
    expect(wrapper.get('[role="status"]').text()).toBe('Empty')
    expect(wrapper.get('button').text()).toBe('Recalculate size')
  })

  it('explains partial lower bounds with counts and skipped entries', async () => {
    useDirectorySizesStore().entries['/pub/docs'] = { status: 'partial', result: { ...result, complete: false, skippedCount: 1, reason: 'unreadable' } }
    const wrapper = await mountSize(true)
    expect(wrapper.get('[role="status"]').text()).toBe('≥ 2,048 B (Partial)')
    expect(wrapper.get('[role="status"]').attributes('title')).toContain('lower bound')
    expect(wrapper.text()).toContain('3 files, 2 folders')
    expect(wrapper.text()).toContain('Skipped entries: 1')
  })

  it('provides a safe localized error and a retry action', async () => {
    const sizes = useDirectorySizesStore()
    sizes.entries['/pub/docs'] = { status: 'error', errorCode: 'ERR_OPERATION_IN_PROGRESS' }
    const calculate = vi.spyOn(sizes, 'calculate').mockResolvedValue()
    const wrapper = await mountSize(true)
    expect(wrapper.text()).toContain('Another size calculation is still running')
    expect(wrapper.get('button').text()).toBe('Retry size calculation')
    await wrapper.get('button').trigger('click')
    expect(calculate).toHaveBeenCalledWith('/pub/docs')
  })

  it('uses the same cached total in list rows, cards and Properties', async () => {
    useDirectorySizesStore().entries['/pub/docs'] = { status: 'complete', result }
    const file = { name: 'docs', isDir: true, size: 999, modified: '2026-01-01T00:00:00Z', mode: 'drwxr-xr-x' }
    const props = { file, selected: false, currentPath: '/pub', editing: false, isCut: false, active: false, compact: false, index: 0, menuItems: [] }
    const row = await mountSuspended(FileRow, { props: { ...props, showPermissions: false }, global: { plugins: [pinia] } })
    wrappers.push(row)
    expect(row.text()).toContain('2,048 B')
    const card = await mountSuspended(FileCard, { props, global: { plugins: [pinia] } })
    wrappers.push(card)
    expect(card.text()).toBe('docs2,048 B')

    useFilesStore(pinia).currentPath = '/pub'
    useModalStore(pinia).open('properties', { file })
    const properties = await mountSuspended(PropertiesModal, {
      global: { plugins: [pinia], stubs: { UModal: { props: ['open', 'title'], template: '<div><slot name="body" /></div>' } } },
    })
    wrappers.push(properties)
    expect(properties.text()).toContain('2,048 B')
    expect(properties.text()).toContain('3 files, 2 folders')
    expect(properties.text()).toContain('Recalculate size')
  })

  it('does not call a directory with excluded entries empty, even when both counts are zero', async () => {
    useDirectorySizesStore().entries['/pub/docs'] = { status: 'complete', result: { ...result, bytes: 0, hasEntries: true, fileCount: 0, directoryCount: 0 } }
    const wrapper = await mountSize(true)
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Empty')
    expect(wrapper.text()).toContain('0 files, 0 folders')
  })

  it('retains a visible last total and counts while recalculating or showing a retryable error', async () => {
    const sizes = useDirectorySizesStore()
    sizes.entries['/pub/docs'] = { status: 'loading', result }
    const wrapper = await mountSize(true)
    expect(wrapper.get('[role="status"]').text()).toBe('2,048 B')
    expect(wrapper.get('[role="status"]').attributes('aria-label')).toContain('Calculating...')
    expect(wrapper.text()).toContain('3 files, 2 folders')
    expect(wrapper.get('button').text()).toBe('Cancel size calculation')

    sizes.entries['/pub/docs'] = { status: 'error', result, errorCode: 'ERR_OPERATION_IN_PROGRESS' }
    await nextTick()
    expect(wrapper.get('[role="status"]').text()).toBe('2,048 B')
    expect(wrapper.text()).toContain('3 files, 2 folders')
    expect(wrapper.text()).toContain('Another size calculation is still running')
    expect(wrapper.get('button').text()).toBe('Retry size calculation')
  })

  it('keeps incomplete zero-byte results marked partial even without observed entries', async () => {
    useDirectorySizesStore().entries['/pub/docs'] = { status: 'partial', result: { ...result, bytes: 0, hasEntries: false, complete: false, reason: 'timeout' } }
    const wrapper = await mountSize()
    expect(wrapper.get('[role="status"]').text()).toBe('Partial')
    expect(wrapper.text()).not.toContain('Empty')
  })
})
