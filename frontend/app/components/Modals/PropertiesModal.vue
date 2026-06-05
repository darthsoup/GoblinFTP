<script setup lang="ts">
const modalStore = useModalStore()
const filesStore = useFilesStore()
const authStore = useAuthStore()
const { t, locale } = useI18n()

const file = computed(() => modalStore.context.file)

const name = ref('')
const octal = ref('')
const initialOctal = ref('')
const loading = ref(false)
const error = ref<string | null>(null)

watch(() => modalStore.active, (v) => {
  if (v === 'properties' && file.value) {
    name.value = file.value.name
    initialOctal.value = modeToOctal(file.value.mode)
    octal.value = initialOctal.value
    error.value = null
  }
})

const iconDef = computed(() => (file.value ? getFileIcon(file.value) : null))

const fullPath = computed(() => {
  if (!file.value)
    return ''
  const dir = filesStore.currentPath.replace(/\/$/, '')
  return `${dir}/${file.value.name}`
})

const chmodEnabled = computed(() =>
  !authStore.systemVars?.connection.disableChmod && !!file.value && !file.value.isDir,
)

// ── Permissions: octal string is the single source of truth ──────────────────
const octalValid = computed(() => /^[0-7]{3,4}$/.test(octal.value.trim()))

function octalDigits(): [number, number, number] | null {
  const s = octal.value.trim()
  if (!/^[0-7]{3,4}$/.test(s))
    return null
  const last3 = s.slice(-3)
  return [Number(last3[0]!), Number(last3[1]!), Number(last3[2]!)]
}

function permChecked(group: number, bit: number): boolean {
  const d = octalDigits()
  return d ? (d[group]! & bit) !== 0 : false
}

function togglePerm(group: number, bit: number) {
  const d = octalDigits() ?? [0, 0, 0]
  d[group] = d[group]! ^ bit
  const trimmed = octal.value.trim()
  const prefix = /^[0-7]{4}$/.test(trimmed) ? trimmed[0]! : ''
  octal.value = `${prefix}${d[0]}${d[1]}${d[2]}`
}

const PERM_GROUPS = [
  { key: 'owner', index: 0 },
  { key: 'group', index: 1 },
  { key: 'public', index: 2 },
] as const
const PERM_BITS = [
  { key: 'read', bit: 4 },
  { key: 'write', bit: 2 },
  { key: 'execute', bit: 1 },
] as const

// ── Apply (rename and/or chmod) ───────────────────────────────────────────────
const nameChanged = computed(() =>
  !!file.value && name.value.trim() !== '' && name.value.trim() !== file.value.name,
)
const permsChanged = computed(() =>
  chmodEnabled.value && octalValid.value && octal.value.trim() !== initialOctal.value,
)
const canApply = computed(() => nameChanged.value || permsChanged.value)

async function apply() {
  if (!file.value || !canApply.value || loading.value)
    return
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const oldPath = `${dir}/${file.value.name}`
  loading.value = true
  error.value = null
  try {
    // chmod first (targets the current name), then rename
    if (permsChanged.value)
      await filesStore.chmod(oldPath, Number.parseInt(octal.value.trim(), 8))
    if (nameChanged.value)
      await filesStore.rename(oldPath, `${dir}/${name.value.trim()}`)
    modalStore.close()
  }
  catch (e) {
    error.value = e instanceof Error ? e.message : t('error.operationFailed')
  }
  finally {
    loading.value = false
  }
}

// ── Display helpers ───────────────────────────────────────────────────────────
function formatSize(bytes: number): string {
  if (file.value?.isDir)
    return '--'
  if (bytes < 1024)
    return `${bytes} B`
  let value: string
  if (bytes < 1048576)
    value = `${(bytes / 1024).toFixed(1)} KB`
  else if (bytes < 1073741824)
    value = `${(bytes / 1048576).toFixed(1)} MB`
  else value = `${(bytes / 1073741824).toFixed(2)} GB`
  return `${value} (${bytes.toLocaleString(locale.value)} B)`
}

function formatDate(iso: string): string {
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime()))
      return iso
    return d.toLocaleString(locale.value || 'en-US')
  }
  catch {
    return iso
  }
}
</script>

<template>
  <UModal :open="modalStore.active === 'properties'" @update:open="modalStore.close()">
    <template #content>
      <div v-if="file" class="flex flex-col min-w-96">
        <!-- Header -->
        <div class="flex items-center justify-between px-4 py-3 border-b border-default bg-elevated/60">
          <h2 class="text-base font-semibold text-highlighted flex items-center gap-2">
            <UIcon name="i-lucide-info" class="size-5 text-muted" />
            {{ t('modal.properties.title') }}
          </h2>
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            icon="i-lucide-x"
            :aria-label="t('modal.properties.cancel')"
            @click="modalStore.close()"
          />
        </div>

        <!-- Body -->
        <div class="p-5 space-y-6">
          <!-- File identity -->
          <div class="flex gap-4 items-start">
            <div class="size-12 rounded bg-default border border-accented flex items-center justify-center shrink-0">
              <UIcon
                v-if="iconDef"
                :name="iconDef.icon"
                class="size-7"
                :class="iconDef.primary ? 'text-primary' : (iconDef.color ? '' : 'text-dimmed')"
                :style="iconDef.color ? { color: iconDef.color } : undefined"
              />
            </div>
            <div class="flex-1 min-w-0 space-y-1">
              <UInput
                v-model="name"
                class="w-full font-mono"
                @keydown.enter="apply"
              />
              <p class="font-mono text-xs text-dimmed truncate" :title="fullPath">
                {{ fullPath }}
              </p>
            </div>
          </div>

          <!-- Meta grid -->
          <div class="grid grid-cols-2 gap-y-3 gap-x-4 bg-elevated/40 p-3 rounded border border-default">
            <div>
              <span class="block label-caps text-muted mb-1">{{ t('modal.properties.size') }}</span>
              <span class="font-mono text-sm text-default">{{ formatSize(file.size) }}</span>
            </div>
            <div>
              <span class="block label-caps text-muted mb-1">{{ t('modal.properties.type') }}</span>
              <span class="font-mono text-sm text-default">{{ file.isDir ? t('modal.properties.typeDir') : t('modal.properties.typeFile') }}</span>
            </div>
            <div>
              <span class="block label-caps text-muted mb-1">{{ t('modal.properties.modified') }}</span>
              <span class="font-mono text-sm text-default">{{ formatDate(file.modified) }}</span>
            </div>
            <div>
              <span class="block label-caps text-muted mb-1">{{ t('modal.properties.permissions') }}</span>
              <span class="font-mono text-sm text-default">{{ file.mode || '--' }}</span>
            </div>
          </div>

          <!-- Permissions (CHMOD) -->
          <div v-if="chmodEnabled">
            <h3 class="label-caps text-muted border-b border-default pb-1 mb-3">
              {{ t('modal.properties.chmodTitle') }}
            </h3>
            <div class="rounded border border-default overflow-hidden">
              <div class="grid grid-cols-4 bg-elevated/60 border-b border-default p-2 label-caps text-muted text-center">
                <div class="text-left pl-2">
                  {{ t('modal.properties.permGroup') }}
                </div>
                <div>{{ t('modal.properties.read') }}</div>
                <div>{{ t('modal.properties.write') }}</div>
                <div>{{ t('modal.properties.execute') }}</div>
              </div>
              <div
                v-for="group in PERM_GROUPS"
                :key="group.key"
                class="grid grid-cols-4 p-2 border-b border-default/50 last:border-b-0 items-center text-center font-mono text-sm"
              >
                <div class="text-left text-default pl-2">
                  {{ t(`modal.properties.${group.key}`) }}
                </div>
                <div v-for="perm in PERM_BITS" :key="perm.key" class="flex justify-center">
                  <input
                    type="checkbox"
                    class="rounded"
                    :checked="permChecked(group.index, perm.bit)"
                    @change="togglePerm(group.index, perm.bit)"
                  >
                </div>
              </div>
            </div>
            <div class="flex items-center gap-3 mt-3 justify-end">
              <label class="label-caps text-muted">{{ t('modal.properties.numeric') }}</label>
              <input
                v-model="octal"
                type="text"
                maxlength="4"
                class="w-16 bg-default border rounded px-2 py-1 font-mono text-sm text-center text-primary focus:border-primary focus:ring-1 focus:ring-primary focus:outline-none transition-colors"
                :class="octalValid || !octal.trim() ? 'border-accented' : 'border-error'"
                @keydown.enter="apply"
              >
            </div>
          </div>

          <UAlert v-if="error" color="error" variant="soft" :description="error" />
        </div>

        <!-- Footer -->
        <div class="flex justify-end gap-2 px-4 py-3 border-t border-default bg-elevated/60">
          <UButton color="neutral" variant="subtle" @click="modalStore.close()">
            {{ t('modal.properties.cancel') }}
          </UButton>
          <UButton :disabled="!canApply" :loading="loading" @click="apply">
            {{ t('modal.properties.apply') }}
          </UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>
