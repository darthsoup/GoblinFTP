export function joinRemotePath(directory: string, name: string): string {
  const dir = directory === '\\' ? '/' : directory
  return `${dir.replace(/\/+$/, '')}/${name}`
}

export function displayName(value: string): string {
  return Array.from(value, (character) => {
    const code = character.codePointAt(0)!
    const unsafe = code < 32 || (code >= 127 && code <= 159) || code === 0x200E || code === 0x200F
      || (code >= 0x202A && code <= 0x202E) || (code >= 0x2066 && code <= 0x2069)
    return unsafe ? `\\u${code.toString(16).padStart(4, '0')}` : character
  }).join('')
}
