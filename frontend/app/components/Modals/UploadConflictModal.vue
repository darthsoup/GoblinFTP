<script setup lang="ts">
import type { UploadConflictAction } from '~/stores/modal'

// Asks how each occupied upload destination should be resolved. Driven by
// modalStore.uploadConflict() (a promise the upload store awaits). Closing via
// backdrop/Esc resolves as 'cancel', which drops the whole batch.
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const { t, locale } = useI18n()

// A large folder drop can conflict on thousands of files; rendering them all
// would stall the dialog, and "apply to all" still covers the hidden ones.
const MAX_ROWS = 200

const conflicts = computed(() => modalStore.uploadConflicts)
const visible = computed(() => conflicts.value.slice(0, MAX_ROWS))
const hiddenCount = computed(() => Math.max(0, conflicts.value.length - MAX_ROWS))

// 'rename' by default: it neither destroys the remote file nor silently drops
// the local one, so an inattentive Continue cannot lose data.
const decisions = ref<Record<string, UploadConflictAction>>({})
watch(conflicts, (entries) => {
  decisions.value = Object.fromEntries(entries.map(e => [e.path, 'rename' as UploadConflictAction]))
}, { immediate: true })

const appliedToAll = ref<UploadConflictAction | undefined>()

const actions = [
  { value: 'overwrite' as const, label: 'modal.uploadConflict.action.overwrite' },
  { value: 'rename' as const, label: 'modal.uploadConflict.action.rename' },
  { value: 'skip' as const, label: 'modal.uploadConflict.action.skip' },
]

function applyToAll(action: UploadConflictAction) {
  appliedToAll.value = action
  for (const entry of conflicts.value) {
    // A directory can never be replaced by an uploaded file.
    decisions.value[entry.path] = entry.isDir && action === 'overwrite' ? 'rename' : action
  }
}

function setDecision(path: string, action: UploadConflictAction) {
  decisions.value[path] = action
  appliedToAll.value = undefined
}

function confirm() {
  modalStore.resolveUploadConflict({
    kind: 'resolve',
    decisions: { ...decisions.value },
    applyToAll: appliedToAll.value,
  })
}

function formatSize(bytes: number): string {
  return formatFileSize(bytes, settingsStore.sizeFormat, locale.value)
}

function formatDate(iso: string): string {
  return formatFileDate(iso, settingsStore.dateFormat, locale.value)
}
</script>

<template>
  <UModal
    :open="modalStore.active === 'uploadConflict'"
    :title="t('modal.uploadConflict.title')"
    :ui="{ content: 'max-w-2xl' }"
    @update:open="(v: boolean) => { if (!v) modalStore.resolveUploadConflict({ kind: 'cancel' }) }"
  >
    <template #body>
      <p class="text-sm text-muted mb-3">
        {{ t('modal.uploadConflict.message', { n: conflicts.length }) }}
      </p>

      <div class="flex items-center gap-2 mb-3 pb-3 border-b border-default">
        <span class="label-caps text-dimmed">{{ t('modal.uploadConflict.applyToAll') }}</span>
        <UButton
          v-for="action in actions"
          :key="action.value"
          size="xs"
          :color="appliedToAll === action.value ? 'primary' : 'neutral'"
          :variant="appliedToAll === action.value ? 'solid' : 'outline'"
          :label="t(action.label)"
          @click="applyToAll(action.value)"
        />
      </div>

      <ul class="max-h-80 overflow-auto space-y-3">
        <li
          v-for="entry in visible"
          :key="entry.path"
          class="rounded border border-default bg-muted/40 px-3 py-2"
        >
          <div class="flex items-center gap-2 mb-1.5">
            <UIcon
              :name="entry.isDir ? 'i-lucide-folder' : 'i-lucide-file'"
              class="size-4 shrink-0 text-dimmed"
            />
            <span class="text-sm text-highlighted truncate">{{ entry.name }}</span>
            <span v-if="!entry.isDir" class="ml-auto shrink-0 text-xs text-dimmed">
              {{ formatSize(entry.size) }} · {{ formatDate(entry.modified) }}
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-1.5">
            <UButton
              v-for="action in actions"
              :key="action.value"
              size="xs"
              :color="decisions[entry.path] === action.value ? 'primary' : 'neutral'"
              :variant="decisions[entry.path] === action.value ? 'solid' : 'ghost'"
              :disabled="entry.isDir && action.value === 'overwrite'"
              :label="t(action.label)"
              @click="setDecision(entry.path, action.value)"
            />
          </div>

          <p v-if="entry.isDir" class="mt-1.5 text-xs text-dimmed">
            {{ t('modal.uploadConflict.folderConflict') }}
          </p>
          <p v-else-if="decisions[entry.path] === 'rename'" class="mt-1.5 text-xs text-dimmed truncate">
            {{ t('modal.uploadConflict.renameHint', { name: entry.suggestedName }) }}
          </p>
        </li>
      </ul>

      <p v-if="hiddenCount > 0" class="mt-2 text-xs text-dimmed">
        {{ t('toast.andMore', { n: hiddenCount }) }}
      </p>
    </template>

    <template #footer>
      <div class="flex w-full items-center justify-end gap-2">
        <UButton
          color="neutral"
          variant="ghost"
          :label="t('modal.uploadConflict.cancel')"
          @click="modalStore.resolveUploadConflict({ kind: 'cancel' })"
        />
        <UButton color="primary" :label="t('modal.uploadConflict.continue')" @click="confirm" />
      </div>
    </template>
  </UModal>
</template>
