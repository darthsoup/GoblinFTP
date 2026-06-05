#!/usr/bin/env node
// Generate a one-time GoblinFTP SSO login link. Node >= 18, stdlib only.
//
// Token format (must match backend/internal/sso/token.go):
//   key   = HKDF-SHA256(secret, salt = empty, info = "gftp-sso", length = 32)
//   token = base64url( iv(12) || gcmTag(16) || AES-256-GCM(key, iv, JSON payload) )
//
// Usage:
//   GFTP_SSO_SECRET=change-me node generate-sso-link.mjs \
//     --host ftp.example.com --username alice --password s3cret \
//     --base-url https://files.example.com

import { createCipheriv, hkdfSync, randomBytes } from 'node:crypto'
import { parseArgs } from 'node:util'

const { values: opts } = parseArgs({
  options: {
    protocol: { type: 'string', default: 'ftp' }, // ftp | sftp
    host: { type: 'string' },
    port: { type: 'string' },
    username: { type: 'string' },
    password: { type: 'string', default: process.env.GFTP_SSO_PASSWORD ?? '' },
    dir: { type: 'string', default: '' }, // initial directory hint
    lang: { type: 'string', default: '' }, // UI language hint (en, de)
    'ttl-seconds': { type: 'string', default: '300' },
    'base-url': { type: 'string', default: 'http://localhost:8080' },
    secret: { type: 'string', default: process.env.GFTP_SSO_SECRET ?? '' },
  },
})

function fail(msg) {
  console.error(`error: ${msg}`)
  process.exit(2)
}

if (!opts.secret) fail('missing --secret (or set GFTP_SSO_SECRET)')
if (!opts.host) fail('missing --host')
if (!opts.username) fail('missing --username')
if (!['ftp', 'sftp'].includes(opts.protocol)) fail('--protocol must be ftp or sftp')

const payload = {
  type: opts.protocol,
  host: opts.host,
  port: Number(opts.port ?? '') || (opts.protocol === 'sftp' ? 22 : 21),
  username: opts.username,
  password: opts.password,
  initialDirectory: opts.dir,
  language: opts.lang,
  exp: Math.floor(Date.now() / 1000) + Number(opts['ttl-seconds']),
}

const key = Buffer.from(
  hkdfSync('sha256', Buffer.from(opts.secret), Buffer.alloc(0), Buffer.from('gftp-sso'), 32),
)
const iv = randomBytes(12)
const cipher = createCipheriv('aes-256-gcm', key, iv)
const ciphertext = Buffer.concat([cipher.update(JSON.stringify(payload), 'utf8'), cipher.final()])
const tag = cipher.getAuthTag()

// Wire format: iv || tag || ciphertext, base64url without padding.
const token = Buffer.concat([iv, tag, ciphertext]).toString('base64url')

console.log(`${opts['base-url'].replace(/\/+$/, '')}/?sso=${token}`)
