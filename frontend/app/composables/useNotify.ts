// Thin wrapper over Nuxt UI's useToast() with consistent success/error styling.
// Requires <UApp> at the root (present in app/app.vue).
export function useNotify() {
  const toast = useToast()
  // `whitespace-pre-line` lets a multi-line description (one failed item per line)
  // render with line breaks.
  const ui = { description: 'whitespace-pre-line' }
  return {
    success: (title: string, description?: string) => toast.add({ title, description, color: 'success', icon: 'i-lucide-check', ui }),
    error: (title: string, description?: string) => toast.add({ title, description, color: 'error', icon: 'i-lucide-triangle-alert', ui }),
  }
}
