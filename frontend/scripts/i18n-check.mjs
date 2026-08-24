#!/usr/bin/env node
// Checks every locale file under i18n/locales for full key + {…} placeholder parity
// with en.json, and that every backend error code has an errorCode.* translation.
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const LOCALES_DIR = join(dirname(fileURLToPath(import.meta.url)), '..', 'i18n', 'locales')
const REFERENCE = 'en.json'
const ERRORS_GO = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'backend', 'internal', 'errors', 'errors.go')

// Codes the SPA never renders: they route to the reconnect dialog or are
// consumed before any message reaches the user. Adding a code here is a
// deliberate choice not to translate it.
const UNRENDERED_CODES = new Set([
  'ERR_SESSION_NOT_FOUND',
  'ERR_UNAUTHORIZED',
  'ERR_CSRF_INVALID',
  'ERR_NOT_IMPLEMENTED',
])

function flatten(obj, prefix = '', out = {}) {
  for (const [key, val] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (val && typeof val === 'object' && !Array.isArray(val))
      flatten(val, path, out)
    else
      out[path] = val
  }
  return out
}

function placeholders(s) {
  return typeof s === 'string' ? [...s.matchAll(/\{[^}]+\}/g)].map(m => m[0]).sort() : []
}

function load(file) {
  return JSON.parse(readFileSync(join(LOCALES_DIR, file), 'utf8'))
}

const ref = flatten(load(REFERENCE))
const refKeys = Object.keys(ref)

const locales = readdirSync(LOCALES_DIR)
  .filter(f => f.endsWith('.json') && f !== REFERENCE)
  .sort()

let failed = false
for (const file of locales) {
  let flat
  try {
    flat = flatten(load(file))
  }
  catch (err) {
    failed = true
    console.log(`✗ ${file}\n  invalid JSON: ${err.message}`)
    continue
  }
  const missing = refKeys.filter(k => !Object.hasOwn(flat, k))
  const extra = Object.keys(flat).filter(k => !Object.hasOwn(ref, k))
  const badPlaceholders = refKeys.filter(k => Object.hasOwn(flat, k) && placeholders(ref[k]).join('') !== placeholders(flat[k]).join(''))

  if (missing.length || extra.length || badPlaceholders.length) {
    failed = true
    console.log(`✗ ${file}`)
    if (missing.length)
      console.log(`  missing (${missing.length}): ${missing.join(', ')}`)
    if (extra.length)
      console.log(`  extra (${extra.length}): ${extra.join(', ')}`)
    for (const k of badPlaceholders)
      console.log(`  placeholder mismatch: ${k} (expected {${placeholders(ref[k]).join(' ')}}, got {${placeholders(flat[k]).join(' ')}})`)
  }
  else {
    console.log(`✓ ${file} (${Object.keys(flat).length} keys)`)
  }
}

// Every code the backend can emit needs a translation, or the user is shown the
// backend's English string (or a bare code) instead.
try {
  const goSource = readFileSync(ERRORS_GO, 'utf8')
  const backendCodes = [...goSource.matchAll(/Code\s*=\s*"(ERR_[A-Z0-9_]+)"/g)].map(m => m[1])
  const enErrorCodes = JSON.parse(readFileSync(join(LOCALES_DIR, REFERENCE), 'utf8')).errorCode ?? {}
  const untranslated = backendCodes.filter(c => !UNRENDERED_CODES.has(c) && !Object.hasOwn(enErrorCodes, c))

  if (untranslated.length) {
    failed = true
    console.log(`✗ ${REFERENCE}`)
    console.log(`  backend codes with no errorCode.* entry (${untranslated.length}): ${untranslated.join(', ')}`)
    console.log('  add them to every locale, or list them in UNRENDERED_CODES if the SPA never shows them')
  }
  else {
    console.log(`✓ all ${backendCodes.length} backend error codes are translated or explicitly unrendered`)
  }
}
catch (err) {
  failed = true
  console.log(`✗ could not cross-check backend error codes: ${err.message}`)
}

if (failed) {
  console.error('\ni18n parity check failed.')
  process.exit(1)
}
console.log(`\nAll ${locales.length} locale(s) match ${REFERENCE} (${refKeys.length} keys).`)
