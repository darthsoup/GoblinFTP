<script setup lang="ts">
import { ApiError } from '~/types/api'

const filter = defineModel<string>('filter', { default: '' })

const filesStore = useFilesStore()
const uploadStore = useUploadStore()
const modalStore = useModalStore()
const settingsStore = useSettingsStore()
const notify = useNotify()
const { t } = useI18n()

const selectedCount = computed(() => filesStore.selected.size)

function clearFilter() {
  filter.value = ''
}

function toggleView() {
  settingsStore.fileViewMode = settingsStore.fileViewMode === 'table' ? 'cards' : 'table'
}

// Hidden file input ref
const fileInputRef = ref<HTMLInputElement | null>(null)

function openNewFolder() {
  modalStore.open('newFolder')
}

function openNewFile() {
  modalStore.open('newFile')
}

function deleteSelected() {
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const paths = [...filesStore.selected].map(name => `${dir}/${name}`)
  modalStore.open('delete', { files: paths })
}

function triggerUpload() {
  fileInputRef.value?.click()
}

async function onFilesSelected(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files || input.files.length === 0)
    return
  // Snapshot before resetting the input — clearing it empties the live FileList,
  // and addFiles now awaits a conflict check before reading it.
  const files = Array.from(input.files)
  // Reset input so the same file can be re-selected later
  input.value = ''
  await uploadStore.addFiles(files, filesStore.currentPath)
}

async function downloadZip() {
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const paths = [...filesStore.selected].map(name => `${dir}/${name}`)
  try {
    await filesStore.downloadZip(paths)
  }
  catch (e) {
    notify.error(e instanceof ApiError ? e.message : t('toast.downloadFailed'))
  }
}

function copySelected() {
  filesStore.copyToClipboard([...filesStore.selected])
}

function cutSelected() {
  filesStore.cutToClipboard([...filesStore.selected])
}

const paste = usePaste()
</script>

<template>
  <div class="flex flex-wrap items-center gap-2 px-4 min-h-10 py-1.5 bg-elevated border-t border-muted border-b border-default shrink-0">
    <!-- Hidden file input for uploads -->
    <input
      ref="fileInputRef"
      type="file"
      multiple
      class="hidden"
      @change="onFilesSelected"
    >

    <!-- Left region swaps between the default command set and a selection mode.
         The filter is anchored right (outside the transition) so it never shifts. -->
    <Transition name="cmd-swap" mode="out-in">
      <!-- Selection mode -->
      <div v-if="selectedCount > 0" key="sel" class="flex flex-wrap items-center gap-2">
        <UBadge color="primary" variant="subtle" size="sm" class="tabular-nums">
          {{ t('toolbar.selected', { n: selectedCount }) }}
        </UBadge>
        <UTooltip :text="t('toolbar.deselectAll')">
          <UButton
            size="sm"
            color="neutral"
            variant="ghost"
            icon="i-lucide-x"
            :aria-label="t('toolbar.deselectAll')"
            @click="filesStore.clearSelection()"
          />
        </UTooltip>

        <USeparator orientation="vertical" class="h-5" />

        <UFieldGroup size="sm">
          <UButton color="neutral" variant="subtle" icon="i-lucide-copy" :aria-label="t('toolbar.copy')" @click="copySelected">
            <span class="hidden md:inline">{{ t('toolbar.copy') }}</span>
          </UButton>
          <UButton color="neutral" variant="subtle" icon="i-lucide-scissors" :aria-label="t('toolbar.cut')" @click="cutSelected">
            <span class="hidden md:inline">{{ t('toolbar.cut') }}</span>
          </UButton>
          <UButton
            v-if="selectedCount >= 2"
            color="neutral"
            variant="subtle"
            icon="i-lucide-file-archive"
            :aria-label="t('toolbar.downloadZip')"
            @click="downloadZip"
          >
            <span class="hidden md:inline">{{ t('toolbar.downloadZip') }}</span>
          </UButton>
        </UFieldGroup>

        <UButton size="sm" color="error" variant="soft" icon="i-lucide-trash-2" @click="deleteSelected">
          {{ t('toolbar.delete') }}
        </UButton>
      </div>

      <!-- Default command set -->
      <div v-else key="def" class="flex flex-wrap items-center gap-2">
        <UButton
          size="sm"
          color="primary"
          icon="i-lucide-folder-plus"
          class="active:translate-y-px"
          @click="openNewFolder"
        >
          {{ t('toolbar.newFolder') }}
        </UButton>
        <UButton
          size="sm"
          color="neutral"
          variant="subtle"
          icon="i-lucide-file-plus"
          class="active:translate-y-px"
          @click="openNewFile"
        >
          <span class="hidden sm:inline">{{ t('toolbar.newFile') }}</span>
        </UButton>

        <USeparator orientation="vertical" class="h-5" />

        <UFieldGroup size="sm">
          <UTooltip :text="t('toolbar.refresh')">
            <UButton
              color="neutral"
              variant="subtle"
              icon="i-lucide-refresh-cw"
              :aria-label="t('toolbar.refresh')"
              :loading="filesStore.loading"
              @click="filesStore.list()"
            />
          </UTooltip>
          <UTooltip :text="t('toolbar.upload')">
            <UButton
              color="neutral"
              variant="subtle"
              icon="i-lucide-upload"
              :aria-label="t('toolbar.upload')"
              @click="triggerUpload"
            />
          </UTooltip>
          <UTooltip :text="t('toolbar.viewToggle')">
            <UButton
              color="neutral"
              variant="subtle"
              :icon="settingsStore.fileViewMode === 'table' ? 'i-lucide-layout-grid' : 'i-lucide-table'"
              :aria-label="t('toolbar.viewToggle')"
              @click="toggleView"
            />
          </UTooltip>
        </UFieldGroup>

        <UButton
          v-if="filesStore.clipboard"
          size="sm"
          color="primary"
          variant="subtle"
          icon="i-lucide-clipboard-paste"
          @click="paste"
        >
          {{ t('toolbar.paste', { n: filesStore.clipboard.names.length }) }}
        </UButton>
      </div>
    </Transition>

    <div class="flex-1" />

    <UInput
      v-model="filter"
      size="sm"
      icon="i-lucide-search"
      :placeholder="t('toolbar.filter')"
      class="w-full sm:w-44 md:w-56"
    >
      <template v-if="filter" #trailing>
        <UButton
          color="neutral"
          variant="link"
          size="xs"
          icon="i-lucide-x"
          :aria-label="t('files.clearFilter')"
          @click="clearFilter"
        />
      </template>
    </UInput>
  </div>
</template>

<style scoped>
/* Left-region swap between default and selection modes — restrained slide+fade. */
.cmd-swap-enter-active,
.cmd-swap-leave-active {
  transition:
    opacity 0.15s ease,
    transform 0.15s ease;
}
.cmd-swap-enter-from {
  opacity: 0;
  transform: translateX(6px);
}
.cmd-swap-leave-to {
  opacity: 0;
  transform: translateX(-6px);
}
@media (prefers-reduced-motion: reduce) {
  .cmd-swap-enter-active,
  .cmd-swap-leave-active {
    transition: none;
  }
}
</style>
