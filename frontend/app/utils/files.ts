import type { FileInfo } from '~/types/api'

export interface FileIconDef {
  icon: string
  color?: string
  primary: boolean
}

// File-type icons with brand colors (folders are Goblin Green)
const FILE_ICONS: Array<{ exts: string[], icon: string, color?: string }> = [
  { exts: ['js', 'mjs', 'cjs', 'jsx'], icon: 'i-simple-icons-javascript', color: '#f0db4f' },
  { exts: ['ts', 'tsx', 'mts'], icon: 'i-simple-icons-typescript', color: '#3178c6' },
  { exts: ['css', 'scss', 'sass', 'less'], icon: 'i-simple-icons-css', color: '#2965f1' },
  { exts: ['html', 'htm'], icon: 'i-simple-icons-html5', color: '#e34f26' },
  { exts: ['php'], icon: 'i-simple-icons-php', color: '#7a86b8' },
  { exts: ['py'], icon: 'i-simple-icons-python', color: '#3776ab' },
  { exts: ['json', 'yml', 'yaml', 'toml'], icon: 'i-lucide-braces', color: '#e3b341' },
  { exts: ['sql', 'db', 'sqlite'], icon: 'i-lucide-database', color: '#aac7ff' },
  { exts: ['zip', 'tar', 'gz', 'tgz', 'bz2', 'xz', 'rar', '7z'], icon: 'i-lucide-file-archive', color: '#e3b341' },
  { exts: ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'bmp'], icon: 'i-lucide-image', color: '#c297ff' },
  { exts: ['mp4', 'mov', 'mkv', 'avi', 'webm'], icon: 'i-lucide-film', color: '#c297ff' },
  { exts: ['mp3', 'wav', 'flac', 'ogg', 'm4a'], icon: 'i-lucide-music', color: '#f778ba' },
  { exts: ['sh', 'bash', 'zsh', 'fish'], icon: 'i-lucide-terminal', color: '#67df70' },
  { exts: ['md', 'markdown', 'txt', 'log', 'conf', 'ini', 'env', 'cfg'], icon: 'i-lucide-file-text' },
]

export function getFileIcon(file: Pick<FileInfo, 'name' | 'isDir'>): FileIconDef {
  if (file.isDir)
    return { icon: 'i-lucide-folder', primary: true }
  const ext = file.name.split('.').pop()?.toLowerCase() ?? ''
  const match = FILE_ICONS.find(d => d.exts.includes(ext))
  return { icon: match?.icon ?? 'i-lucide-file', color: match?.color, primary: false }
}

// ── Preview ───────────────────────────────────────────────────────────────────
// Largest file we'll pull over FTP into the browser for an inline preview. Above
// this the panel shows "too large" + a download button instead. Text uses the
// read endpoint's own 1 MB server cap.
export const PREVIEW_MAX_BYTES = 5 * 1024 * 1024

export type PreviewKind = 'image' | 'video' | 'audio' | 'pdf' | 'text' | 'none'

// Extension → MIME, used to build correctly-typed object URLs for media the
// download endpoint serves as application/octet-stream.
const PREVIEW_MIME: Record<string, string> = {
  // image
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  bmp: 'image/bmp',
  ico: 'image/x-icon',
  svg: 'image/svg+xml',
  avif: 'image/avif',
  // video
  mp4: 'video/mp4',
  m4v: 'video/mp4',
  webm: 'video/webm',
  ogv: 'video/ogg',
  mov: 'video/quicktime',
  // audio
  mp3: 'audio/mpeg',
  wav: 'audio/wav',
  ogg: 'audio/ogg',
  oga: 'audio/ogg',
  m4a: 'audio/mp4',
  aac: 'audio/aac',
  flac: 'audio/flac',
  opus: 'audio/opus',
  // documents
  pdf: 'application/pdf',
}

const IMAGE_EXTS = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'ico', 'svg', 'avif']
const VIDEO_EXTS = ['mp4', 'm4v', 'webm', 'ogv', 'mov']
const AUDIO_EXTS = ['mp3', 'wav', 'ogg', 'oga', 'm4a', 'aac', 'flac', 'opus']

function extOf(name: string): string {
  return name.split('.').pop()?.toLowerCase() ?? ''
}

// Classifies a file for the preview panel. `textExts` is the editor's
// allowed-extension whitelist (the read endpoint only serves those as text).
export function getPreviewKind(file: Pick<FileInfo, 'name' | 'isDir'>, textExts: string[]): PreviewKind {
  if (file.isDir)
    return 'none'
  const ext = extOf(file.name)
  if (IMAGE_EXTS.includes(ext))
    return 'image'
  if (VIDEO_EXTS.includes(ext))
    return 'video'
  if (AUDIO_EXTS.includes(ext))
    return 'audio'
  if (ext === 'pdf')
    return 'pdf'
  if (textExts.includes(ext))
    return 'text'
  return 'none'
}

// MIME type for an extension, or '' when unknown.
export function previewMime(name: string): string {
  return PREVIEW_MIME[extOf(name)] ?? ''
}

// Largest image we'll fetch full-size for a card-grid thumbnail (there's no
// server-side resize). Bigger images fall back to the type icon.
export const THUMBNAIL_MAX_BYTES = 3 * 1024 * 1024

export function isImageFile(name: string): boolean {
  return IMAGE_EXTS.includes(extOf(name))
}

// Parse "drwxr-xr-x" → "755"; returns '' when the mode string is unknown/unparseable
export function modeToOctal(mode: string): string {
  const perms = mode.slice(1, 10)
  if (perms.length < 9)
    return ''
  function triplet(r: string, w: string, x: string): number {
    return (r !== '-' ? 4 : 0) + (w !== '-' ? 2 : 0) + (x !== '-' ? 1 : 0)
  }
  const u = triplet(perms[0]!, perms[1]!, perms[2]!)
  const g = triplet(perms[3]!, perms[4]!, perms[5]!)
  const o = triplet(perms[6]!, perms[7]!, perms[8]!)
  return `${u}${g}${o}`
}
