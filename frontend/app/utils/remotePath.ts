export function remotePath(path: string): string {
  const parts: string[] = []
  for (const part of path.split('/')) {
    if (part === '..')
      parts.pop()
    else if (part && part !== '.')
      parts.push(part)
  }
  return `/${parts.join('/')}`
}

export function parentPath(path: string): string {
  const normalized = remotePath(path)
  return normalized.slice(0, normalized.lastIndexOf('/')) || '/'
}

export function pathContains(parent: string, path: string): boolean {
  return parent === '/' || path === parent || path.startsWith(`${parent}/`)
}

export function ancestorPaths(path: string): string[] {
  const parts = remotePath(path).split('/').filter(Boolean)
  return ['/', ...parts.map((_, i) => `/${parts.slice(0, i + 1).join('/')}`)]
}
