import { describe, expect, it } from 'vitest'
import { getPreviewKind, previewMime } from '~/utils/files'

describe('getPreviewKind', () => {
  const textExts = ['md', 'txt', 'json', 'ts']

  it('classifies media by extension (case-insensitive)', () => {
    expect(getPreviewKind({ name: 'photo.png', isDir: false }, textExts)).toBe('image')
    expect(getPreviewKind({ name: 'PHOTO.JPG', isDir: false }, textExts)).toBe('image')
    expect(getPreviewKind({ name: 'clip.mp4', isDir: false }, textExts)).toBe('video')
    expect(getPreviewKind({ name: 'song.mp3', isDir: false }, textExts)).toBe('audio')
    expect(getPreviewKind({ name: 'doc.pdf', isDir: false }, textExts)).toBe('pdf')
  })

  it('treats editable extensions as text, others as none', () => {
    expect(getPreviewKind({ name: 'readme.md', isDir: false }, textExts)).toBe('text')
    expect(getPreviewKind({ name: 'data.json', isDir: false }, textExts)).toBe('text')
    expect(getPreviewKind({ name: 'archive.zip', isDir: false }, textExts)).toBe('none')
    expect(getPreviewKind({ name: 'noext', isDir: false }, textExts)).toBe('none')
  })

  it('renders SVG as an image even when it is also an editable extension', () => {
    expect(getPreviewKind({ name: 'logo.svg', isDir: false }, [...textExts, 'svg'])).toBe('image')
  })

  it('never previews directories', () => {
    expect(getPreviewKind({ name: 'folder.png', isDir: true }, textExts)).toBe('none')
  })
})

describe('previewMime', () => {
  it('maps known extensions to a MIME type', () => {
    expect(previewMime('a.mp4')).toBe('video/mp4')
    expect(previewMime('a.pdf')).toBe('application/pdf')
    expect(previewMime('a.MP3')).toBe('audio/mpeg')
    expect(previewMime('a.png')).toBe('image/png')
  })

  it('returns an empty string for unknown extensions', () => {
    expect(previewMime('a.bin')).toBe('')
    expect(previewMime('noext')).toBe('')
  })
})
