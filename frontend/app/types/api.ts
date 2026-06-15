export interface ApiEnvelope<T> {
  success: boolean
  data?: T
  errors?: Array<{ code: string, message: string }>
}

export interface AuthStatus {
  connected: boolean
  ssoAutoConnect: boolean
  csrfToken: string
  // Present only when connected — used to restore SPA state after a reload.
  host?: string
  initialDirectory?: string
  capabilities?: { disableChmod: boolean }
}

export interface ConnectRequest {
  protocol: string
  host: string
  port: number
  username: string
  password: string
  passive: boolean
  // SHA256 fingerprint the user agreed to trust for an unknown SFTP host
  // (trust-on-first-use, step 2). Omitted on the first attempt.
  acceptHostKey?: string
}

export interface HostKeyPrompt {
  // Bare host the key belongs to (shown in the confirmation prompt).
  host: string
  fingerprint: string
  keyType: string
}

export interface ConnectData {
  capabilities: { disableChmod: boolean }
  initialDirectory: string
  csrfToken: string
  // Set (with the other fields empty and no session yet) when an SFTP host key
  // must be confirmed before connecting.
  hostKeyPrompt?: HostKeyPrompt
}

export interface FileInfo {
  name: string
  size: number
  isDir: boolean
  modified: string // RFC3339
  mode: string // e.g., "drwxr-xr-x"
}

export interface SystemVars {
  language: string
  ui: {
    pageTitle: string
    showDotFiles: boolean
    showNavigationHistory: boolean
  }
  branding: {
    appName: string
    logoUrl: string | null
    faviconUrl: string | null
    primaryColor: string | null
    tagline: string | null
    hideAttribution: boolean
  }
  upload: {
    chunkSize: number
    maxConcurrentUploads: number
  }
  connection: {
    allowedTypes: string[]
    disableChmod: boolean
    presetHost: string | null
    presetPort: number | null
    lockHost: boolean
    passiveMode: boolean
  }
  editor: {
    disabled: boolean
    viewOnly: boolean
    allowedExtensions: string[]
  }
  loginFormDisabled: boolean
  ssoEnabled: boolean
  frontendLogEnabled: boolean
  version: string
}

export class ApiError extends Error {
  constructor(public code: string, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}
