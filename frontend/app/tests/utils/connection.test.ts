import { mockNuxtImport } from '@nuxt/test-utils/runtime'
import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useApi } from '~/composables/useApi'
import { invalidateConnection } from '~/utils/connection'

const { fetchMock, rawMock } = vi.hoisted(() => {
  const fetchMock = Object.assign(vi.fn(), { raw: vi.fn() })
  return { fetchMock, rawMock: fetchMock.raw }
})
mockNuxtImport('$fetch', () => fetchMock)

describe('connection-scoped requests', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
    invalidateConnection()
    fetchMock.mockReset()
    rawMock.mockReset()
  })

  it('stops previews using actual bytes even when Content-Length is smaller', async () => {
    const cancel = vi.fn().mockResolvedValue(undefined)
    const read = vi.fn().mockResolvedValue({ done: false, value: new Uint8Array(6) })
    rawMock.mockResolvedValue({
      headers: new Headers({ 'Content-Length': '1' }),
      _data: { getReader: () => ({ read, cancel }) },
    })
    await expect(useApi().getBlob('/preview', 5)).rejects.toMatchObject({ code: 'ERR_FILE_TOO_LARGE' })
    expect(cancel).toHaveBeenCalledOnce()
    expect(rawMock.mock.calls[0]![1].signal.aborted).toBe(true)
  })

  it('ignores a response that completes after its connection was retired', async () => {
    let resolve!: (result: unknown) => void
    fetchMock.mockImplementation(() => {
      return new Promise((done) => {
        resolve = done
      })
    })
    const pending = useApi().get('/api/files')
    invalidateConnection()
    resolve({ success: true, data: ['old server'] })
    await expect(pending).rejects.toMatchObject({ code: 'ERR_STALE_CONNECTION' })
  })

  it('cancels pending dialog resolvers and rejects stale modal origins', async () => {
    const auth = useAuthStore()
    auth.connectionId = 'server-a'
    const modal = useModalStore()
    modal.open('delete', { files: ['/important'] })
    expect(modal.isCurrent()).toBe(true)
    auth.connectionId = 'server-b'
    expect(modal.isCurrent()).toBe(false)
    const confirmation = modal.confirm({ title: 't', message: 'm', confirmLabel: 'yes', cancelLabel: 'no' })
    const paste = modal.pasteConflict(['file'])
    modal.cancelAll()
    await expect(confirmation).resolves.toBe('cancel')
    await expect(paste).resolves.toBe('cancel')
    expect(modal.active).toBeNull()
    expect(modal.context).toEqual({})
  })
})
