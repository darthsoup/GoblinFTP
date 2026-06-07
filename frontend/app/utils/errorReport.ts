// Pure helpers for the browser error reporter (plugins/error-reporter.client.ts).
// Kept free of Nuxt/Pinia imports so they are unit-testable.

export type FrontendErrorKind = 'error' | 'rejection' | 'vue'

export interface FrontendErrorPayload {
  kind: FrontendErrorKind
  message: string
  stack?: string
  source?: string
  route?: string
}

// Mirror the backend's server-side limits (internal/api/frontendlog.go) so a
// report never trips the 16K body limit.
const MESSAGE_MAX = 500
const STACK_MAX = 4000
const FIELD_MAX = 500

export function truncate(value: string | undefined, max: number): string | undefined {
  if (value === undefined)
    return undefined
  return value.length > max ? value.slice(0, max) : value
}

/** Extracts a message/stack from whatever was thrown and truncates all fields. */
export function buildErrorPayload(
  kind: FrontendErrorKind,
  err: unknown,
  route?: string,
  source?: string,
): FrontendErrorPayload {
  let message: string
  let stack: string | undefined

  if (err instanceof Error) {
    message = err.message || err.name
    stack = err.stack
  }
  else if (typeof err === 'string') {
    message = err
  }
  else {
    message = String(err)
  }

  return {
    kind,
    message: truncate(message, MESSAGE_MAX) ?? '',
    stack: truncate(stack, STACK_MAX),
    source: truncate(source, FIELD_MAX),
    route: truncate(route, FIELD_MAX),
  }
}

/** Stable identity for client-side dedupe — one report per distinct error. */
export function errorDedupeKey(p: FrontendErrorPayload): string {
  return `${p.kind}:${p.message}:${p.source ?? ''}`
}
