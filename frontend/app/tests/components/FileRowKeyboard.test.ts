import type { FileInfo } from '~/types/api'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import { createTestingPinia } from '@pinia/testing'
import { setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FileRow from '~/components/FileBrowser/FileRow.vue'

function file(over: Partial<FileInfo> = {}): FileInfo {
  return { name: 'report.pdf', size: 10, isDir: false, modified: '2026-01-01T00:00:00Z', mode: '-rw-r--r--', ...over }
}

async function mountRow(over: Partial<FileInfo> = {}, index = 0) {
  return mountSuspended(FileRow, {
    props: {
      file: file(over),
      selected: false,
      currentPath: '/pub',
      editing: false,
      isCut: false,
      active: false,
      compact: false,
      showPermissions: false,
      index,
      menuItems: [],
    },
  })
}

// The file list was a bare <tr @click> with no tabindex, role or keydown, so a
// keyboard-only user could not open a folder, preview a file, or select
// anything - a WCAG 2.1.1 failure for the app's primary surface.
describe('fileRow keyboard operability', () => {
  beforeEach(() => {
    setActivePinia(createTestingPinia({ createSpy: vi.fn, stubActions: false }))
  })

  it('puts the first row in the tab order and the rest out of it', async () => {
    const first = await mountRow({}, 0)
    const later = await mountRow({}, 3)
    expect(first.find('tr').attributes('tabindex')).toBe('0')
    expect(later.find('tr').attributes('tabindex')).toBe('-1')
  })

  it('opens a directory on Enter', async () => {
    const w = await mountRow({ name: 'docs', isDir: true })
    await w.find('tr').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('navigate')?.[0]).toEqual(['/pub/docs'])
  })

  it('previews a file on Enter', async () => {
    const w = await mountRow()
    await w.find('tr').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('preview')).toHaveLength(1)
  })

  it('selects on Space without activating', async () => {
    const w = await mountRow()
    await w.find('tr').trigger('keydown', { key: ' ' })
    expect(w.emitted('select')?.[0]).toEqual(['report.pdf'])
    expect(w.emitted('preview')).toBeUndefined()
  })

  it('arrow keys, Home and End request focus movement', async () => {
    const w = await mountRow()
    const tr = w.find('tr')
    await tr.trigger('keydown', { key: 'ArrowDown' })
    await tr.trigger('keydown', { key: 'ArrowUp' })
    await tr.trigger('keydown', { key: 'Home' })
    await tr.trigger('keydown', { key: 'End' })
    expect(w.emitted('focusMove')).toEqual([[1], [-1], ['first'], ['last']])
  })

  it('starts a rename on F2 for files but not directories', async () => {
    const onFile = await mountRow()
    await onFile.find('tr').trigger('keydown', { key: 'F2' })
    expect(onFile.emitted('requestRename')).toHaveLength(1)

    const onDir = await mountRow({ name: 'docs', isDir: true })
    await onDir.find('tr').trigger('keydown', { key: 'F2' })
    expect(onDir.emitted('requestRename')).toBeUndefined()
  })

  it('names the row and its checkbox for screen readers', async () => {
    const w = await mountRow()
    const tr = w.find('tr')
    // Previously the checkbox announced "report.pdf, checkbox" - the file name,
    // not what activating it does.
    expect(tr.attributes('aria-label')).toContain('report.pdf')
    expect(tr.attributes('aria-selected')).toBe('false')
    const checkboxLabel = w.find('[role="checkbox"]').attributes('aria-label')
      ?? w.find('input[type="checkbox"]').attributes('aria-label')
    expect(checkboxLabel).toBeTruthy()
    expect(checkboxLabel).not.toBe('report.pdf')
  })
})
