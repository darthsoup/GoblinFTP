import { Storage } from 'happy-dom'

// Node 24+ defines its own web-storage globals that stay undefined without
// --localstorage-file, and they shadow the ones the test environment provides.
for (const name of ['localStorage', 'sessionStorage'] as const) {
  if (!globalThis[name]) {
    Object.defineProperty(globalThis, name, { value: new Storage(), configurable: true, writable: true })
  }
}
