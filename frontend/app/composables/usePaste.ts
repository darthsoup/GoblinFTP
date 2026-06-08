import { ApiError } from '~/types/api'

// Paste the files-store clipboard into the current directory and surface the
// outcome as a toast. Shared by the context menu, the toolbar button, and the
// Ctrl/Cmd+V shortcut so the feedback stays consistent.
export function usePaste() {
  const filesStore = useFilesStore()
  const notify = useNotify()
  const { t } = useI18n()

  return async function paste(): Promise<void> {
    try {
      const res = await filesStore.paste()
      if (res.cancelled)
        return
      if (res.ok > 0)
        notify.success(res.mode === 'copy' ? t('toast.copied', { n: res.ok }) : t('toast.moved', { n: res.ok }))
      if (res.failed > 0)
        notify.error(t('toast.pasteFailed'))
    }
    catch (e) {
      notify.error(e instanceof ApiError ? e.message : t('toast.pasteFailed'))
    }
  }
}
