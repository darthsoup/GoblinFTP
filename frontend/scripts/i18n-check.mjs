#!/usr/bin/env node
// Checks every locale file under i18n/locales for full key + {…} placeholder parity with en.json.
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const LOCALES_DIR = join(dirname(fileURLToPath(import.meta.url)), '..', 'i18n', 'locales')
const REFERENCE = 'en.json'

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

if (failed) {
  console.error('\ni18n parity check failed.')
  process.exit(1)
}
console.log(`\nAll ${locales.length} locale(s) match ${REFERENCE} (${refKeys.length} keys).`)
