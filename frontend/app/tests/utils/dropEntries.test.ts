import { describe, expect, it } from 'vitest'
import { readDropEntries } from '~/utils/dropEntries'

function fileEntry(name: string): FileSystemEntry {
  return {
    isFile: true,
    isDirectory: false,
    name,
    file: (cb: (f: File) => void) => cb(new File(['x'], name)),
  } as unknown as FileSystemEntry
}

// A directory whose reader yields `batches` in order, then an empty batch — this
// exercises the readEntries() paging loop.
function dirEntry(name: string, batches: FileSystemEntry[][]): FileSystemEntry {
  let i = 0
  return {
    isFile: false,
    isDirectory: true,
    name,
    createReader: () => ({
      readEntries: (ok: (e: FileSystemEntry[]) => void) => ok(i < batches.length ? batches[i++]! : []),
    }),
  } as unknown as FileSystemEntry
}

function dataTransfer(entries: FileSystemEntry[], looseFiles: File[] = []): DataTransfer {
  return {
    items: entries.map(e => ({ kind: 'file', webkitGetAsEntry: () => e })),
    files: looseFiles,
  } as unknown as DataTransfer
}

describe('readDropEntries', () => {
  it('traverses a nested tree, builds relative paths, and reports empty dirs', async () => {
    const tree = dirEntry('project', [[
      fileEntry('a.js'),
      dirEntry('src', [[fileEntry('b.js')]]),
      dirEntry('logs', [[]]), // empty subfolder
    ]])

    const { files, emptyDirs } = await readDropEntries(dataTransfer([tree]))

    expect(files.map(f => f.relativePath).sort()).toEqual(['project/a.js', 'project/src/b.js'])
    expect(emptyDirs).toEqual(['project/logs'])
  })

  it('honors the readEntries paging loop (multiple batches)', async () => {
    const paged = dirEntry('d', [[fileEntry('1.txt')], [fileEntry('2.txt')]])

    const { files } = await readDropEntries(dataTransfer([paged]))

    expect(files.map(f => f.relativePath).sort()).toEqual(['d/1.txt', 'd/2.txt'])
  })

  it('treats a dropped loose file (via entry) as a top-level file', async () => {
    const { files, emptyDirs } = await readDropEntries(dataTransfer([fileEntry('x.txt')]))

    expect(files).toHaveLength(1)
    expect(files[0]!.relativePath).toBe('x.txt')
    expect(emptyDirs).toEqual([])
  })

  it('falls back to the flat FileList when entries are unavailable', async () => {
    const loose = new File(['x'], 'flat.txt')
    const { files, emptyDirs } = await readDropEntries(dataTransfer([], [loose]))

    expect(files).toEqual([{ file: loose, relativePath: 'flat.txt' }])
    expect(emptyDirs).toEqual([])
  })
})
