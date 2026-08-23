import { describe, expect, it } from 'vitest'
import { scrubUrl } from '~/utils/sentryScrub'

describe('scrubUrl', () => {
  it('redacts the download token but keeps the request readable', () => {
    const out = scrubUrl('/api/files/download?path=%2Fpub%2Fa.txt&token=DOWNLOAD_TOKEN')
    expect(out).not.toContain('DOWNLOAD_TOKEN')
    expect(out).toContain('/api/files/download')
    expect(out).toContain('token=[Filtered]')
  })

  it('redacts the SSO token, which decrypts to the FTP password', () => {
    expect(scrubUrl('https://gftp.example/?sso=SSO_TOKEN')).toBe('https://gftp.example/?sso=[Filtered]')
  })

  it('redacts remote file paths, which name customer data', () => {
    const out = scrubUrl('https://gftp.example/edit?path=/customers/acme/invoices.csv')
    expect(out).not.toContain('acme')
    expect(out).toContain('path=[Filtered]')
  })

  it('leaves harmless params and query-less URLs alone', () => {
    expect(scrubUrl('/api/files?sort=name')).toBe('/api/files?sort=name')
    expect(scrubUrl('/api/system/vars')).toBe('/api/system/vars')
    expect(scrubUrl(undefined)).toBeUndefined()
  })

  it('preserves the fragment', () => {
    expect(scrubUrl('/edit?token=SECRET#L42')).toBe('/edit?token=[Filtered]#L42')
  })

  it('matches param names case-insensitively and as substrings', () => {
    const out = scrubUrl('/x?X-CSRF-Token=A&sessionId=B&apiKey=C')
    expect(out).not.toMatch(/=A|=B|=C/)
  })
})
