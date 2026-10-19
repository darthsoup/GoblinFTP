import { describe, expect, it } from 'vitest'
import { displayName, joinRemotePath } from '~/utils/remotePath'

describe('remote paths and labels', () => {
  it('joins Windows roots while preserving literal filename characters', () => {
    expect(joinRemotePath('\\', 'file')).toBe('/file')
    expect(joinRemotePath('/', 'a\\b')).toBe('/a\\b')
    expect(joinRemotePath('/dir/', 'file')).toBe('/dir/file')
  })
  it('makes direction and control characters visible without changing ordinary Unicode', () => {
    expect(displayName('invoice\u202Etxt.exe')).toBe('invoice\\u202etxt.exe')
    expect(displayName('a\n\u2066b')).toBe('a\\u000a\\u2066b')
    expect(displayName('Größe_日本語.txt')).toBe('Größe_日本語.txt')
  })
})
