import type { MaybeRefOrGetter } from 'vue'

export function useDirectorySize(path: MaybeRefOrGetter<string>) {
  const sizes = useDirectorySizesStore()
  const settings = useSettingsStore()
  const { t, locale } = useI18n()
  const { localizeError } = useErrorMessage()
  const entry = computed(() => sizes.get(toValue(path)))
  const result = computed(() => entry.value.result)
  const loading = computed(() => entry.value.status === 'loading')
  const reason = computed(() => result.value?.reason ? t(`directorySize.reason.${result.value.reason}`) : '')
  const explanation = computed(() => {
    if (entry.value.status === 'error')
      return localizeError(entry.value.errorCode ?? '', t('directorySize.failed'))
    return reason.value
  })
  const label = computed(() => {
    if (!result.value) {
      if (loading.value)
        return t('directorySize.calculating')
      return entry.value.status === 'error' ? t('directorySize.failed') : ''
    }
    if (result.value.bytes === 0) {
      if (!result.value.complete)
        return t('directorySize.partial')
      return result.value.hasEntries ? '' : t('directorySize.empty')
    }
    const size = formatFileSize(result.value.bytes, settings.sizeFormat, locale.value)
    return result.value.complete ? size : `≥ ${size} (${t('directorySize.partial')})`
  })
  const actionLabel = computed(() => {
    if (loading.value)
      return t('directorySize.cancel')
    if (entry.value.status === 'error')
      return t('directorySize.retry')
    return t(result.value ? 'directorySize.recalculate' : 'directorySize.calculate')
  })

  function act() {
    if (loading.value)
      sizes.cancel()
    else
      return sizes.calculate(toValue(path))
  }

  return { entry, result, loading, reason, explanation, label, actionLabel, act }
}
