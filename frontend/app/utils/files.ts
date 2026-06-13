import type { FileInfo } from '~/types/api'

export interface FileIconDef {
  icon: string
  color?: string
  primary: boolean
}

// File-type icons with brand colors (folders are Goblin Green). First match in
// .find() wins, so keep extensions non-overlapping. Icons come only from the two
// offline-installed sets (@iconify-json/lucide, @iconify-json/simple-icons) —
// arbitrary Iconify collections won't render. Colors apply identically in light
// and dark mode, so avoid near-black/white brand hexes that vanish on one theme.
// Exported for the icon-prefix sanity test in tests/utils/files.test.ts.
export const FILE_ICONS: Array<{ exts: string[], icon: string, color?: string }> = [
  // Web / markup
  { exts: ['html', 'htm'], icon: 'i-simple-icons-html5', color: '#e34f26' },
  { exts: ['css', 'scss', 'sass', 'less'], icon: 'i-simple-icons-css', color: '#2965f1' },
  { exts: ['xml', 'xsl', 'xsd'], icon: 'i-lucide-file-code', color: '#e3b341' },
  // JS / TS / Vue
  { exts: ['js', 'mjs', 'cjs', 'jsx'], icon: 'i-simple-icons-javascript', color: '#f0db4f' },
  { exts: ['ts', 'tsx', 'mts'], icon: 'i-simple-icons-typescript', color: '#3178c6' },
  { exts: ['vue'], icon: 'i-simple-icons-vuedotjs', color: '#42b883' },
  // Languages
  { exts: ['py'], icon: 'i-simple-icons-python', color: '#5a9fd4' },
  { exts: ['php'], icon: 'i-simple-icons-php', color: '#8993be' },
  { exts: ['go'], icon: 'i-simple-icons-go', color: '#00add8' },
  { exts: ['rs'], icon: 'i-simple-icons-rust', color: '#f74c00' }, // brand is black → orange
  { exts: ['rb'], icon: 'i-simple-icons-ruby', color: '#cc342d' },
  { exts: ['java', 'jar', 'class'], icon: 'i-simple-icons-openjdk', color: '#ed8b00' },
  { exts: ['c', 'h'], icon: 'i-simple-icons-c', color: '#659ad2' },
  { exts: ['cpp', 'cc', 'cxx', 'hpp', 'hh'], icon: 'i-simple-icons-cplusplus', color: '#659ad2' },
  { exts: ['cs'], icon: 'i-simple-icons-csharp', color: '#8a52d4' },
  { exts: ['kt', 'kts'], icon: 'i-simple-icons-kotlin', color: '#a97bff' },
  { exts: ['swift'], icon: 'i-simple-icons-swift', color: '#f05138' },
  { exts: ['dart'], icon: 'i-simple-icons-dart', color: '#2bb7f6' },
  { exts: ['lua'], icon: 'i-simple-icons-lua', color: '#6d8ce8' },
  { exts: ['graphql', 'gql'], icon: 'i-simple-icons-graphql', color: '#e535ab' },
  { exts: ['wasm'], icon: 'i-simple-icons-webassembly', color: '#654ff0' },
  // Shell / scripts
  { exts: ['sh', 'bash', 'zsh', 'fish', 'bat', 'cmd', 'ps1'], icon: 'i-lucide-terminal', color: '#67df70' },
  // Data / config
  { exts: ['json', 'yml', 'yaml', 'toml'], icon: 'i-lucide-braces', color: '#e3b341' },
  { exts: ['sql', 'db', 'sqlite'], icon: 'i-lucide-database', color: '#aac7ff' },
  // Documents
  { exts: ['pdf'], icon: 'i-simple-icons-adobeacrobatreader', color: '#ec1c24' },
  { exts: ['doc', 'docx', 'rtf', 'odt'], icon: 'i-simple-icons-microsoftword', color: '#2b579a' },
  { exts: ['xls', 'xlsx', 'ods'], icon: 'i-simple-icons-microsoftexcel', color: '#217346' },
  { exts: ['ppt', 'pptx', 'odp'], icon: 'i-simple-icons-microsoftpowerpoint', color: '#d24726' },
  { exts: ['csv', 'tsv'], icon: 'i-lucide-file-spreadsheet', color: '#21a366' },
  { exts: ['epub', 'mobi', 'azw3'], icon: 'i-lucide-book', color: '#c297ff' },
  // Media
  { exts: ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'bmp', 'avif'], icon: 'i-lucide-image', color: '#c297ff' },
  { exts: ['mp4', 'mov', 'mkv', 'avi', 'webm', 'm4v'], icon: 'i-lucide-film', color: '#c297ff' },
  { exts: ['mp3', 'wav', 'flac', 'ogg', 'm4a', 'aac', 'opus'], icon: 'i-lucide-music', color: '#f778ba' },
  // Fonts
  { exts: ['ttf', 'otf', 'woff', 'woff2', 'eot'], icon: 'i-lucide-type', color: '#c297ff' },
  // Archives / disk images / packages
  { exts: ['zip', 'tar', 'gz', 'tgz', 'bz2', 'xz', 'rar', '7z'], icon: 'i-lucide-file-archive', color: '#e3b341' },
  { exts: ['iso'], icon: 'i-lucide-disc', color: '#aac7ff' },
  { exts: ['deb', 'rpm', 'appimage', 'snap', 'flatpak'], icon: 'i-simple-icons-linux', color: '#fcc624' },
  // Executables / installers
  { exts: ['exe', 'msi'], icon: 'i-simple-icons-windows', color: '#0078d6' },
  { exts: ['dmg', 'pkg', 'app'], icon: 'i-simple-icons-apple', color: '#b0b8c1' }, // brand is black → gray
  { exts: ['bin', 'dat'], icon: 'i-lucide-binary', color: '#9aa3b2' },
  // Keys / certs
  { exts: ['pem', 'crt', 'cer', 'key', 'pub', 'p12', 'pfx'], icon: 'i-lucide-file-key', color: '#aac7ff' },
  // Docker / git (extensionless names resolve via .split('.').pop())
  { exts: ['dockerfile'], icon: 'i-simple-icons-docker', color: '#2496ed' },
  { exts: ['gitignore', 'gitattributes', 'gitmodules'], icon: 'i-simple-icons-git', color: '#f05032' },
  // Plain text / misc (no color → dimmed)
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
