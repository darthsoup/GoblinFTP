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
  // Set when a DIFFERENT key was pinned before (server reinstalled — or MITM);
  // confirming replaces the pin instead of adding a first-trust entry.
  changed?: boolean
  oldFingerprint?: string
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

// A single per-item failure in a batch/multi-item operation. `code` is a stable
// classifier code (localizable via errorCode.*); `message` is the server's
// friendly fallback.
export interface OperationFailure {
  path: string
  code: string
  message: string
}

// One occupied upload destination, from POST /api/files/upload/check. Only
// conflicting paths come back, each with a free name the server picked by
// listing the target directory.
export interface UploadConflict {
  path: string
  name: string
  suggestedName: string
  size: number
  isDir: boolean
  modified: string
}

// Optimistic-concurrency token from /api/files/read and /api/files/write.
// `version` is opaque: never parse or reconstruct it, just hand it back on save.
// null means the server could not stat the path, so there is no conflict
// detection for this file and the editor saves unconditionally.
export interface FileVersion {
  version: string | null
  size: number
  modified: string
}

export interface ReadFileResult extends FileVersion {
  path: string
  content: string
}

// Result of DELETE /api/files — the request succeeds (HTTP 200) once processed;
// per-item outcomes live here.
export interface DeleteResult {
  deleted: string[]
  failed: OperationFailure[]
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
    logoDarkUrl: string | null
    faviconUrl: string | null
    primaryColor: string | null
    primaryTextColor: string | null
    hideAttribution: boolean
    themeCssUrl: string | null
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
