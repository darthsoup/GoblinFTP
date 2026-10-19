import type { VueWrapper } from '@vue/test-utils'
import type { FileInfo } from '~/types/api'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import { flushPromises } from '@vue/test-utils'
import { createPinia, getActivePinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DirectoryTree from '~/components/FileBrowser/DirectoryTree.vue'
import { ApiError } from '~/types/api'

const mockApi = { get: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

function folder(name: string, isDir = true): FileInfo {
  return { name, isDir, size: 0, modified: '', mode: '' }
}

describe('directory tree with Nuxt UI', () => {
  let wrapper: VueWrapper | undefined

  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    useAuthStore().connected = true
    vi.resetAllMocks()
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(480)
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(256)
    mockApi.get.mockImplementation((url: string) => Promise.resolve(url === '/api/files?path=%2F' ? [folder('docs'), folder('.hidden'), folder('file.txt', false)] : []))
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.restoreAllMocks()
  })

  async function mountTree(): Promise<VueWrapper> {
    await useFilesStore().list('/')
    wrapper = await mountSuspended(DirectoryTree, { attachTo: document.body, global: { plugins: [getActivePinia()!] } })
    await flushPromises()
    return wrapper
  }

  it('shows only folders and leaves unopened branches unloaded', async () => {
    const tree = await mountTree()
    expect(tree.find('[role="tree"]').exists()).toBe(true)
    expect(tree.text()).toContain('docs')
    expect(tree.text()).not.toContain('file.txt')
    expect(tree.text()).not.toContain('.hidden')
    expect(mockApi.get).toHaveBeenCalledTimes(1)
    expect(tree.findAll('[role="treeitem"]').find(row => row.find('[data-disclosure="/docs"]').exists())!.attributes('aria-expanded')).toBe('false')
  })

  it('loads an unknown folder with its disclosure without navigating and removes expansion for empty folders', async () => {
    const tree = await mountTree()
    await tree.find('[data-disclosure="/docs"]').trigger('click')
    await flushPromises()
    expect(mockApi.get).toHaveBeenCalledTimes(2)
    expect(useFilesStore().currentPath).toBe('/')
    expect(tree.find('[data-disclosure="/docs"]').exists()).toBe(false)
    const docs = tree.findAll('[role="treeitem"]').find(row => row.text().includes('docs'))!
    expect(docs.attributes('aria-expanded')).toBeUndefined()
  })

  it('supports keyboard expansion, collapse, and directory navigation', async () => {
    mockApi.get.mockImplementation((url: string) => Promise.resolve(url === '/api/files?path=%2F' ? [folder('docs')] : [folder('nested')]))
    const tree = await mountTree()
    const docs = tree.findAll('[role="treeitem"]').find(row => row.find('[data-disclosure="/docs"]').exists())!
    await docs.trigger('keydown', { key: 'ArrowRight' })
    await flushPromises()
    expect(tree.text()).toContain('nested')
    expect(useFilesStore().currentPath).toBe('/')
    await docs.trigger('keydown', { key: 'ArrowLeft' })
    await flushPromises()
    expect(tree.text()).not.toContain('nested')
    await docs.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(useFilesStore().currentPath).toBe('/docs')
    expect(tree.emitted('navigated')).toHaveLength(1)
    expect(useFilesStore().canGoBack).toBe(true)
  })

  it('renders a translated branch error as a keyboard-accessible retry without navigating', async () => {
    const tree = await mountTree()
    mockApi.get.mockRejectedValueOnce(new ApiError('ERR_FILE_PERMISSION', 'raw server detail'))
    await tree.find('[data-disclosure="/docs"]').trigger('click')
    await flushPromises()
    const retry = tree.findAll('[role="treeitem"]').find(row => row.text().includes('Retry'))!
    expect(retry).toBeTruthy()
    expect(tree.text()).not.toContain('raw server detail')
    await retry.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(useDirectoriesStore().listings['/docs']?.errorCode).toBeNull()
    expect(useFilesStore().currentPath).toBe('/')
  })

  it('reveals an accessible current directory even when its parent cannot be listed', async () => {
    await useFilesStore().list('/private/home')
    mockApi.get.mockRejectedValue(new ApiError('ERR_FILE_PERMISSION', 'denied'))
    wrapper = await mountSuspended(DirectoryTree, { attachTo: document.body, global: { plugins: [getActivePinia()!] } })
    await flushPromises()
    expect(wrapper.text()).toContain('private')
    expect(wrapper.text()).toContain('home')
    const active = wrapper.findAll('[role="treeitem"]').find(row => row.attributes('aria-selected') === 'true')!
    expect(active.text()).toContain('home')
  })

  it('keeps a hidden current ancestor visible and follows the dotfile preference for other folders', async () => {
    await useFilesStore().list('/.hidden')
    wrapper = await mountSuspended(DirectoryTree, { attachTo: document.body, global: { plugins: [getActivePinia()!] } })
    await flushPromises()
    expect(wrapper.text()).toContain('.hidden')
    await useFilesStore().navigate('/')
    await flushPromises()
    expect(wrapper.text()).not.toContain('.hidden')
    useSettingsStore().showDotfiles = true
    await flushPromises()
    expect(wrapper.text()).toContain('.hidden')
  })

  it('refreshes an invalidated visible branch but waits to reload a collapsed branch', async () => {
    const tree = await mountTree()
    await tree.find('[data-disclosure="/docs"]').trigger('click')
    await flushPromises()
    useDirectoriesStore().expanded = ['/']
    useDirectoriesStore().invalidate(['/docs/new'])
    await flushPromises()
    expect(useDirectoriesStore().listings['/docs']?.dirty).toBe(true)
    await tree.find('[data-disclosure="/docs"]').trigger('click')
    await flushPromises()
    expect(useDirectoriesStore().listings['/docs']?.dirty).toBe(false)
  })
})
