import type { ConnectRequest } from '~/types/api'
import { mockNuxtImport } from '@nuxt/test-utils/runtime'
import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mockApi = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), del: vi.fn() }
vi.mock('~/composables/useApi', () => ({ useApi: () => mockApi }))

const { fetchMock } = vi.hoisted(() => ({ fetchMock: vi.fn() }))
mockNuxtImport('$fetch', () => fetchMock)

const req: ConnectRequest = {
  protocol: 'sftp',
  host: 'ssh.example.com',
  port: 22,
  username: 'u',
  password: 'p',
  passive: false,
}

// ofetch REJECTS on any non-2xx, so the old `if (!res.success)` branch was
// unreachable and every failure fell through to a hardcoded "Connection failed".
// These tests reject, which is why they catch what the existing ones could not.
function rejectWith(code: string, message: string) {
  const err = Object.assign(new Error(`[POST] "/api/auth/connect": 401`), {
    status: 401,
    data: { success: false, errors: [{ code, message }] },
  })
  fetchMock.mockRejectedValue(err)
}

describe('useAuthStore error propagation', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    vi.clearAllMocks()
  })

  it('surfaces the real code and message from a rejected connect', async () => {
    rejectWith('ERR_AUTH_FAILED', 'the server rejected the username or password')
    const store = useAuthStore()

    await expect(store.connect({ ...req })).rejects.toThrow()

    expect(store.errorCode).toBe('ERR_AUTH_FAILED')
    expect(store.error).toBe('the server rejected the username or password')
    expect(store.error).not.toBe('Connection failed')
  })

  it('surfaces a host-key mismatch instead of a generic failure', async () => {
    rejectWith('ERR_HOST_KEY_MISMATCH', 'the server\'s host key does not match the trusted one')
    const store = useAuthStore()

    await expect(store.connect({ ...req })).rejects.toThrow()

    // This is the man-in-the-middle warning. It was previously unreachable.
    expect(store.errorCode).toBe('ERR_HOST_KEY_MISMATCH')
    expect(store.error).toContain('host key')
  })

  it('reports a TLS failure with its own code', async () => {
    rejectWith('ERR_TLS_FAILED', 'the server\'s TLS certificate could not be verified')
    const store = useAuthStore()

    await expect(store.connect({ ...req })).rejects.toThrow()
    expect(store.errorCode).toBe('ERR_TLS_FAILED')
  })

  it('falls back to a network code when there is no envelope at all', async () => {
    fetchMock.mockRejectedValue(Object.assign(new Error('[POST] "/api/auth/connect": 502 Bad Gateway'), {
      status: 502,
    }))
    const store = useAuthStore()

    await expect(store.connect({ ...req })).rejects.toThrow()
    expect(store.errorCode).toBe('ERR_NETWORK')
  })

  it('flags a failed bootstrap so the screen can offer a retry', async () => {
    fetchMock.mockRejectedValue(new Error('network down'))
    const store = useAuthStore()

    await store.init()

    expect(store.bootFailed).toBe(true)
  })

  it('clears bootFailed once the bootstrap succeeds', async () => {
    fetchMock.mockResolvedValue({ success: true, data: { connected: false, ssoAutoConnect: false, csrfToken: '' } })
    const store = useAuthStore()

    await store.init()

    expect(store.bootFailed).toBe(false)
  })
})
