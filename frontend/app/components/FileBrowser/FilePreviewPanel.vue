<script setup lang="ts">
import type { FileInfo } from '~/types/api'
import { ApiError } from '~/types/api'

const props = defineProps<{
  file: FileInfo
  dir: string
}>()

const emit = defineEmits<{
  close: []
}>()

const filesStore = useFilesStore()
const authStore = useAuthStore()
const settingsStore = useSettingsStore()
const modalStore = useModalStore()
const notify = useNotify()
const { t, locale } = useI18n()

// Fallback text extensions when systemVars hasn't loaded yet (the read endpoint
// still gates on the server's allowed list, so this is just for classification).
const FALLBACK_TEXT_EXTS = ['txt', 'md', 'markdown', 'log', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'conf', 'cfg', 'env', 'csv', 'sql', 'sh', 'js', 'ts', 'css', 'html', 'htm', 'py', 'go', 'rb', 'php']

const fullPath = computed(() => `${props.dir.replace(/\/$/, '')}/${props.file.name}`)
const iconDef = computed(() => getFileIcon(props.file))
const textExts = computed(() => authStore.systemVars?.editor.allowedExtensions ?? FALLBACK_TEXT_EXTS)
const kind = computed(() => getPreviewKind(props.file, textExts.value))

const ext = computed(() => props.file.name.split('.').pop()?.toLowerCase() ?? '')
const editEnabled = computed(() => {
  const ed = authStore.systemVars?.editor
  if (!ed || ed.disabled || props.file.isDir)
    return false
  return ed.allowedExtensions.some(a => a.toLowerCase() === ext.value)
})

// ── Preview loading ───────────────────────────────────────────────────────────
type Status = 'loading' | 'ready' | 'tooLarge' | 'none' | 'error'
const status = ref<Status>('loading')
const mediaUrl = ref<string | null>(null)
const textContent = ref<string | null>(null)
let urlIsBlob = false
let reqId = 0

function revoke() {
  if (urlIsBlob && mediaUrl.value)
    URL.revokeObjectURL(mediaUrl.value)
  urlIsBlob = false
}

async function load() {
  const id = ++reqId
  revoke()
  mediaUrl.value = null
  textContent.value = null
  status.value = 'loading'

  const k = kind.value
  if (k === 'none') {
    status.value = 'none'
    return
  }
  // Binary kinds are size-capped (text relies on the read endpoint's 1 MB cap).
  if (k !== 'text' && props.file.size > PREVIEW_MAX_BYTES) {
    status.value = 'tooLarge'
    return
  }

  const path = fullPath.value
  try {
    if (k === 'text') {
      const data = await useApi().get<{ content: string }>(`/api/files/read?path=${encodeURIComponent(path)}`)
      if (id !== reqId)
        return
      textContent.value = data.content
    }
    else {
      // image/video/audio/pdf: fetch as a typed object URL. The download endpoint
      // serves octet-stream, so a typed blob is needed for reliable rendering
      // (e.g. SVG won't render from an octet-stream <img> in some browsers).
      const url = await filesStore.fetchObjectUrl(path, previewMime(props.file.name))
      if (id !== reqId) {
        URL.revokeObjectURL(url) // a newer request superseded us — don't leak
        return
      }
      mediaUrl.value = url
      urlIsBlob = true
    }
    status.value = 'ready'
  }
  catch (e) {
    if (id !== reqId)
      return
    status.value = e instanceof ApiError && e.code === 'ERR_FILE_TOO_LARGE' ? 'tooLarge' : 'error'
  }
}

// Key the reload on the path, not the file object, so a list refresh that
// replaces the object without changing the target doesn't refetch.
watch(fullPath, load, { immediate: true })
onBeforeUnmount(revoke)

// ── Actions ───────────────────────────────────────────────────────────────────
async function download() {
  try {
    await filesStore.downloadFile(fullPath.value)
  }
  catch (e) {
    notify.error(e instanceof ApiError ? e.message : t('toast.downloadFailed'))
  }
}

function edit() {
  navigateTo({ path: '/edit', query: { path: fullPath.value } })
}

function openProperties() {
  modalStore.open('properties', { file: props.file })
}

function fmtSize(bytes: number): string {
  return formatFileSize(bytes, settingsStore.sizeFormat, locale.value)
}

function fmtDate(iso: string): string {
  return formatFileDate(iso, settingsStore.dateFormat, locale.value)
}
</script>

<template>
  <aside class="flex flex-col overflow-hidden border-l border-default bg-default md:bg-elevated/30">
    <!-- Header -->
    <div class="flex items-center gap-2 px-3 h-11 border-b border-default shrink-0">
      <UIcon
        :name="iconDef.icon"
        class="size-5 shrink-0"
        :class="iconDef.primary ? 'text-primary' : (iconDef.color ? '' : 'text-dimmed')"
        :style="iconDef.color ? { color: iconDef.color } : undefined"
      />
      <span class="flex-1 min-w-0 truncate font-mono text-sm text-highlighted" :title="fullPath">{{ file.name }}</span>
      <UButton
        size="sm"
        color="neutral"
        variant="ghost"
        icon="i-lucide-x"
        :aria-label="t('preview.close')"
        @click="emit('close')"
      />
    </div>

    <div class="flex-1 overflow-auto">
      <!-- Preview region -->
      <div class="p-3">
        <div
          v-if="status === 'loading'"
          class="flex items-center justify-center min-h-40 rounded border border-default bg-muted/30 text-muted"
        >
          <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-primary" />
        </div>

        <div
          v-else-if="status === 'error'"
          class="flex flex-col items-center justify-center gap-2 min-h-40 rounded border border-default bg-muted/30 text-muted font-mono text-sm"
        >
          <UIcon name="i-lucide-triangle-alert" class="size-7 text-error" />
          {{ t('preview.error') }}
        </div>

        <div
          v-else-if="status === 'tooLarge' || status === 'none'"
          class="flex flex-col items-center justify-center gap-3 min-h-40 rounded border border-default bg-muted/30 text-muted font-mono text-sm"
        >
          <UIcon :name="status === 'tooLarge' ? 'i-lucide-weight' : 'i-lucide-eye-off'" class="size-7 text-dimmed" />
          {{ status === 'tooLarge' ? t('preview.tooLarge') : t('preview.noPreview') }}
          <UButton size="xs" color="neutral" variant="subtle" icon="i-lucide-download" @click="download">
            {{ t('context.download') }}
          </UButton>
        </div>

        <template v-else>
          <img
            v-if="kind === 'image' && mediaUrl"
            :src="mediaUrl"
            :alt="file.name"
            class="block w-full max-h-80 object-contain rounded border border-default bg-muted/30"
          >
          <video
            v-else-if="kind === 'video' && mediaUrl"
            :src="mediaUrl"
            controls
            class="block w-full max-h-80 rounded border border-default bg-black"
          />
          <div
            v-else-if="kind === 'audio' && mediaUrl"
            class="flex flex-col items-center gap-3 p-4 rounded border border-default bg-muted/30"
          >
            <UIcon name="i-lucide-music" class="size-10 text-dimmed" />
            <audio :src="mediaUrl" controls class="w-full" />
          </div>
          <iframe
            v-else-if="kind === 'pdf' && mediaUrl"
            :src="mediaUrl"
            :title="file.name"
            class="block w-full h-96 rounded border border-default bg-white"
          />
          <pre
            v-else-if="kind === 'text' && textContent !== null"
            class="w-full max-h-96 overflow-auto rounded border border-default bg-muted p-2 text-xs font-mono text-default whitespace-pre"
          >{{ textContent }}</pre>
        </template>
      </div>

      <!-- Metadata -->
      <div class="px-3 pb-3">
        <div class="grid grid-cols-2 gap-y-3 gap-x-4 bg-elevated/40 p-3 rounded border border-default">
          <div>
            <span class="block label-caps text-muted mb-1">{{ t('modal.properties.size') }}</span>
            <span class="font-mono text-sm text-default">{{ fmtSize(file.size) }}</span>
          </div>
          <div>
            <span class="block label-caps text-muted mb-1">{{ t('modal.properties.type') }}</span>
            <span class="font-mono text-sm text-default">{{ file.isDir ? t('modal.properties.typeDir') : t('modal.properties.typeFile') }}</span>
          </div>
          <div>
            <span class="block label-caps text-muted mb-1">{{ t('modal.properties.modified') }}</span>
            <span class="font-mono text-sm text-default">{{ fmtDate(file.modified) }}</span>
          </div>
          <div>
            <span class="block label-caps text-muted mb-1">{{ t('modal.properties.permissions') }}</span>
            <span class="font-mono text-sm text-default">{{ file.mode || '--' }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Actions -->
    <div class="flex items-center gap-2 px-3 py-2 border-t border-default shrink-0">
      <UButton size="sm" color="primary" variant="subtle" icon="i-lucide-download" @click="download">
        {{ t('context.download') }}
      </UButton>
      <UButton
        v-if="editEnabled"
        size="sm"
        color="neutral"
        variant="subtle"
        icon="i-lucide-pencil"
        @click="edit"
      >
        {{ authStore.systemVars?.editor?.viewOnly ? t('context.view') : t('context.edit') }}
      </UButton>
      <div class="flex-1" />
      <UTooltip :text="t('context.properties')">
        <UButton
          size="sm"
          color="neutral"
          variant="ghost"
          icon="i-lucide-info"
          :aria-label="t('context.properties')"
          @click="openProperties"
        />
      </UTooltip>
    </div>
  </aside>
</template>
