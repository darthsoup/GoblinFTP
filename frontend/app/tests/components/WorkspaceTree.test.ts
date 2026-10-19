import type { VueWrapper } from '@vue/test-utils'
import { mockNuxtImport, mountSuspended } from '@nuxt/test-utils/runtime'
import { flushPromises } from '@vue/test-utils'
import { createPinia, getActivePinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import Workspace from '~/pages/index.vue'

const media = vi.hoisted(() => ({ desktop: true }))
const routing = vi.hoisted(() => ({ query: {} as Record<string, string> }))
mockNuxtImport('useMediaQuery', () => (query: string) => ref(query === '(min-width: 1024px)' && media.desktop))
mockNuxtImport('useRoute', () => () => routing)
const mockApi = { get: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

describe('workspace tree layout', () => {
  let wrapper: VueWrapper | undefined

  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    const auth = useAuthStore()
    auth.connected = true
    auth.initialDirectory = '/home'
    media.desktop = true
    vi.resetAllMocks()
    mockApi.get.mockResolvedValue([])
    vi.spyOn(useRouter(), 'replace').mockImplementation(async (to) => {
      if (typeof to !== 'string')
        routing.query = to.query as Record<string, string>
    })
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.restoreAllMocks()
  })

  async function mountWorkspace(path?: string) {
    routing.query = path ? { path } : {}
    wrapper = await mountSuspended(Workspace, {
      global: {
        plugins: [getActivePinia()!],
        stubs: {
          AppHeader: true,
          FileTable: true,
          UploadProgressPanel: true,
          StatusBar: true,
          UTooltip: { template: '<slot />' },
          DirectoryTree: { emits: ['navigated'], template: '<button data-tree @click="$emit(\'navigated\')">Folder navigation</button>' },
          USlideover: { props: ['open'], template: '<div v-if="open" data-drawer><slot name="body" /></div>' },
        },
      },
    })
    return wrapper
  }

  it('waits for the initial directory listing before mounting the async tree', async () => {
    let finish!: (files: []) => void
    mockApi.get.mockReturnValue(new Promise<[]>((resolve) => {
      finish = resolve
    }))
    const page = await mountWorkspace()
    expect(page.find('[data-tree]').exists()).toBe(false)
    expect(mockApi.get).toHaveBeenCalledExactlyOnceWith('/api/files?path=%2Fhome')
    finish([])
    await flushPromises()
    expect(page.find('aside [data-tree]').exists()).toBe(true)
  })

  it('restores the URL directory and synchronizes successful tree navigation with the URL', async () => {
    const page = await mountWorkspace('/home/docs')
    await flushPromises()
    expect(mockApi.get).toHaveBeenCalledWith('/api/files?path=%2Fhome%2Fdocs')
    expect(useFilesStore().currentPath).toBe('/home/docs')
    expect(page.find('[data-tree]').exists()).toBe(true)
    await useFilesStore().navigate('/elsewhere')
    await flushPromises()
    expect(routing.query.path).toBe('/elsewhere')
    await useFilesStore().goBack()
    await flushPromises()
    expect(routing.query.path).toBe('/home/docs')
  })

  it('persists the desktop visibility toggle', async () => {
    const page = await mountWorkspace()
    await flushPromises()
    await page.find('[aria-controls="directory-tree"]').trigger('click')
    expect(page.find('aside').exists()).toBe(false)
    await flushPromises()
    expect(JSON.parse(localStorage.getItem('gftp_settings')!).directoryTreeVisible).toBe(false)
  })

  it('opens a mobile drawer and closes it after successful navigation without changing the desktop preference', async () => {
    media.desktop = false
    const page = await mountWorkspace()
    await flushPromises()
    expect(page.find('aside').exists()).toBe(false)
    expect(page.find('[data-drawer]').exists()).toBe(false)
    await page.find('[aria-controls="directory-tree"]').trigger('click')
    expect(page.find('[data-drawer] [data-tree]').exists()).toBe(true)
    await page.find('[data-tree]').trigger('click')
    expect(page.find('[data-drawer]').exists()).toBe(false)
    expect(useSettingsStore().directoryTreeVisible).toBe(true)
  })
})
