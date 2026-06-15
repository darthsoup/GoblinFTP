// Recursively read files (and folders) from a drag-and-drop. The
// DataTransferItemList is only valid synchronously during the drop event, so
// webkitGetAsEntry() must be called before any await — we snapshot the entries
// first, then traverse them asynchronously.

export interface DroppedFile {
  file: File
  relativePath: string // POSIX, e.g. "project/src/app.js"
}

export interface DropResult {
  files: DroppedFile[]
  // Directories with no file beneath them — these need explicit creation;
  // non-empty dirs are created implicitly by the upload (backend mkdir -p).
  emptyDirs: string[]
}

function getFile(entry: FileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject))
}

// readEntries() yields at most ~100 entries per call; loop until it drains.
function readAll(reader: FileSystemDirectoryReader): Promise<FileSystemEntry[]> {
  return new Promise((resolve, reject) => {
    const all: FileSystemEntry[] = []
    const next = () => reader.readEntries((batch) => {
      if (batch.length === 0) {
        resolve(all)
        return
      }
      all.push(...batch)
      next()
    }, reject)
    next()
  })
}

async function walk(entry: FileSystemEntry, prefix: string, files: DroppedFile[], dirs: Set<string>): Promise<void> {
  if (entry.isFile) {
    const file = await getFile(entry as FileSystemFileEntry)
    files.push({ file, relativePath: prefix + entry.name })
    return
  }
  if (entry.isDirectory) {
    const dirPath = prefix + entry.name
    dirs.add(dirPath)
    const children = await readAll((entry as FileSystemDirectoryEntry).createReader())
    for (const child of children)
      await walk(child, `${dirPath}/`, files, dirs)
  }
}

// readDropEntries turns a drop's DataTransfer into a flat list of files carrying
// their nested relative paths, plus the set of empty directories to preserve.
// Falls back to the flat FileList when the File System Entries API is unavailable.
export async function readDropEntries(dt: DataTransfer): Promise<DropResult> {
  // Snapshot entries synchronously — the items list is invalid after this tick.
  const entries: FileSystemEntry[] = []
  for (const item of Array.from(dt.items)) {
    if (item.kind !== 'file')
      continue
    const entry = item.webkitGetAsEntry?.()
    if (entry)
      entries.push(entry)
  }

  if (entries.length === 0) {
    return {
      files: Array.from(dt.files).map(file => ({ file, relativePath: file.name })),
      emptyDirs: [],
    }
  }

  const files: DroppedFile[] = []
  const dirs = new Set<string>()
  for (const entry of entries)
    await walk(entry, '', files, dirs)

  const emptyDirs = [...dirs].filter(d => !files.some(f => f.relativePath.startsWith(`${d}/`)))
  return { files, emptyDirs }
}
