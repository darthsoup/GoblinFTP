// Query-string redaction for Sentry payloads. Kept free of Nuxt/Sentry imports so
// it is unit-testable, and mirrored by the backend's key-name scrubbing.

// Matched case-insensitively against the whole param name. `path` is here because
// a remote file path names the customer's data, not because it is a credential.
const SENSITIVE_PARAMS = ['sso', 'token', 'session', 'csrf', 'auth', 'password', 'secret', 'key', 'path']

const REDACTED = '[Filtered]'

function isSensitive(name: string): boolean {
  const lower = name.toLowerCase()
  return SENSITIVE_PARAMS.some(p => lower.includes(p))
}

/**
 * Replaces the values of sensitive query params in a URL, leaving everything else
 * readable. Returns the input unchanged when it has no query string, and drops the
 * query entirely if the URL cannot be parsed (never returns raw secrets).
 */
export function scrubUrl(url: string | undefined): string | undefined {
  if (!url)
    return url
  const queryStart = url.indexOf('?')
  if (queryStart === -1)
    return url

  const base = url.slice(0, queryStart)
  const rest = url.slice(queryStart + 1)
  // Keep the fragment out of the param walk; it is preserved verbatim.
  const hashStart = rest.indexOf('#')
  const query = hashStart === -1 ? rest : rest.slice(0, hashStart)
  const hash = hashStart === -1 ? '' : rest.slice(hashStart)

  try {
    const params = new URLSearchParams(query)
    let changed = false
    for (const name of [...params.keys()]) {
      if (isSensitive(name)) {
        params.set(name, REDACTED)
        changed = true
      }
    }
    if (!changed)
      return url
    // URLSearchParams re-encodes, so [Filtered] would arrive percent-escaped.
    return `${base}?${params.toString().replace(/%5BFiltered%5D/gi, REDACTED)}${hash}`
  }
  catch {
    return `${base}?${REDACTED}${hash}`
  }
}
