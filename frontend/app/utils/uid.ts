/**
 * Random id generator that works outside secure contexts.
 *
 * crypto.randomUUID is only defined on HTTPS and localhost, so a self-hosted
 * deployment reached over plain HTTP on a LAN address threw
 * "crypto.randomUUID is not a function" on the first upload and on opening the
 * first editor tab. crypto.getRandomValues has no such restriction.
 */
export function uid(): string {
  if (typeof crypto.randomUUID === 'function')
    return crypto.randomUUID()

  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  // RFC 4122 version 4 / variant 10xx, so the value still reads as a UUID.
  bytes[6] = (bytes[6]! & 0x0F) | 0x40
  bytes[8] = (bytes[8]! & 0x3F) | 0x80

  const hex = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
