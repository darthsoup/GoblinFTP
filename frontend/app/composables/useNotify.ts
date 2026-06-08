// Thin wrapper over Nuxt UI's useToast() with consistent success/error styling.
// Requires <UApp> at the root (present in app/app.vue).
export function useNotify() {
  const toast = useToast()
  return {
    success: (title: string) => toast.add({ title, color: 'success', icon: 'i-lucide-check' }),
    error: (title: string) => toast.add({ title, color: 'error', icon: 'i-lucide-triangle-alert' }),
  }
}
