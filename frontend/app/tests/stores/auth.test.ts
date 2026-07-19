import type { ConnectData, ConnectRequest } from '~/types/api'
import { mockNuxtImport } from '@nuxt/test-utils/runtime'
import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// ssoConnect goes through useApi (CSRF-aware, returns unwrapped data); connect
// hits $fetch directly (public endpoint, full envelope) — so we mock both.
// $fetch is auto-imported (Nuxt 4.5+), so it's mocked via mockNuxtImport
// instead of by stubbing a global.
const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

const { fetchMock } = vi.hoisted(() => ({ fetchMock: vi.fn() }))
mockNuxtImport('$fetch', () => fetchMock)

const promptData: ConnectData = {
  capabilities: { disableChmod: false },
  initialDirectory: '',
  csrfToken: '',
  hostKeyPrompt: { host: 'ssh.example.com', fingerprint: 'SHA256:abc123', keyType: 'ssh-ed25519' },
}
const successData: ConnectData = {
  capabilities: { disableChmod: false },
  initialDirectory: '/home/user',
  csrfToken: 'csrf-xyz',
}
const req: ConnectRequest = { protocol: 'sftp', host: 'ssh.example.com', port: 22, username: 'u', password: 'p', passive: false }

describe('useAuthStore host-key flow', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
  })

  it('connect() with a host-key prompt pauses without connecting', async () => {
    fetchMock.mockResolvedValue({ success: true, data: promptData })
    const store = useAuthStore()

    await store.connect({ ...req })

    expect(store.connected).toBe(false)
    expect(store.pendingHostKey).toEqual({
      host: 'ssh.example.com',
      fingerprint: 'SHA256:abc123',
      keyType: 'ssh-ed25519',
      sso: false,
    })
  })

  it('confirmHostKey() retries with the accepted fingerprint and connects', async () => {
    fetchMock
      .mockResolvedValueOnce({ success: true, data: promptData })
      .mockResolvedValueOnce({ success: true, data: successData })
    const store = useAuthStore()

    await store.connect({ ...req })
    expect(store.pendingHostKey).not.toBeNull()

    await store.confirmHostKey()

    expect(store.connected).toBe(true)
    expect(store.pendingHostKey).toBeNull()
    expect(fetchMock).toHaveBeenLastCalledWith(
      '/api/auth/connect',
      expect.objectContaining({ body: expect.objectContaining({ acceptHostKey: 'SHA256:abc123' }) }),
    )
  })

  it('cancelHostKey() abandons the pending prompt', async () => {
    fetchMock.mockResolvedValue({ success: true, data: promptData })
    const store = useAuthStore()

    await store.connect({ ...req })
    expect(store.pendingHostKey).not.toBeNull()

    store.cancelHostKey()
    expect(store.pendingHostKey).toBeNull()
  })

  it('ssoConnect() surfaces a host-key prompt flagged sso', async () => {
    mockApi.post.mockResolvedValue(promptData)
    const store = useAuthStore()

    await store.ssoConnect()

    expect(store.connected).toBe(false)
    expect(store.pendingHostKey).toEqual({
      host: 'ssh.example.com',
      fingerprint: 'SHA256:abc123',
      keyType: 'ssh-ed25519',
      sso: true,
    })
  })
})
