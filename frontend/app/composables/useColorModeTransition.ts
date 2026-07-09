// Animate the light/dark flip with the View Transitions API: a circular reveal
// from the toggle (or a plain cross-fade when there's no pointer origin, e.g. the
// settings selector). @nuxtjs/color-mode otherwise swaps the `.dark`/`.light`
// class in a hard cut. Falls back to an instant switch where View Transitions are
// unsupported (Firefox) or the user prefers reduced motion.
interface ViewTransition {
  ready: Promise<void>
  finished: Promise<void>
}
type StartViewTransition = (cb: () => void | Promise<void>) => ViewTransition

export function useColorModeTransition() {
  const colorMode = useColorMode()

  function apply(preference: string, origin?: { x: number, y: number }) {
    if (!import.meta.client) {
      colorMode.preference = preference
      return
    }
    const start = (document as unknown as { startViewTransition?: StartViewTransition }).startViewTransition
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (typeof start !== 'function' || reduce) {
      colorMode.preference = preference
      return
    }

    const root = document.documentElement
    // `.vt-circle` keeps the old snapshot static so the new theme clips in over it
    // (main.css); the default cross-fade is left in place for the no-origin case.
    if (origin)
      root.classList.add('vt-circle')

    // Await nextTick so @nuxtjs/color-mode's (reactive, async) class flip lands
    // BEFORE the API snapshots the new state — otherwise the reveal would show an
    // identical snapshot and the real flip would happen instantly outside it.
    const transition = start.call(document, async () => {
      colorMode.preference = preference
      await nextTick()
    })
    transition.finished.finally(() => root.classList.remove('vt-circle'))

    if (!origin)
      return

    transition.ready
      .then(() => {
        const { x, y } = origin
        const endRadius = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y))
        root.animate(
          { clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${endRadius}px at ${x}px ${y}px)`] },
          { duration: 450, easing: 'ease-in-out', pseudoElement: '::view-transition-new(root)' },
        )
      })
      .catch(() => {})
  }

  // Toggle light<->dark. A real pointer click (detail > 0) reveals from the cursor;
  // keyboard activation cross-fades (no meaningful origin).
  function toggle(event?: MouseEvent) {
    const next = colorMode.value === 'dark' ? 'light' : 'dark'
    const origin = event && event.detail > 0 ? { x: event.clientX, y: event.clientY } : undefined
    apply(next, origin)
  }

  return { apply, toggle }
}
