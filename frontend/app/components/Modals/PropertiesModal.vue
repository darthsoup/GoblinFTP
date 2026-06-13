<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'

const modalStore = useModalStore()
const filesStore = useFilesStore()
const authStore = useAuthStore()
const notify = useNotify()
const { t, locale } = useI18n()

const file = computed(() => modalStore.context.file)
const open = computed({
  get: () => modalStore.active === 'properties',
  set: (v: boolean) => {
    if (!v)
      modalStore.close()
  },
})

const state = reactive({ name: '', octal: '' })
const initialOctal = ref('')
const loading = ref(false)
const apiError = ref<string | null>(null)

watch(open, (v) => {
  if (v && file.value) {
    state.name = file.value.name
    initialOctal.value = modeToOctal(file.value.mode)
    state.octal = initialOctal.value
    apiError.value = null
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
const octalValid = computed(() => /^[0-7]{3,4}$/.test(state.octal.trim()))

function octalDigits(): [number, number, number] | null {
  const s = state.octal.trim()
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
  const trimmed = state.octal.trim()
  const prefix = /^[0-7]{4}$/.test(trimmed) ? trimmed[0]! : ''
  state.octal = `${prefix}${d[0]}${d[1]}${d[2]}`
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

// ── Validation + apply (rename and/or chmod) ──────────────────────────────────
const nameChanged = computed(() =>
  !!file.value && state.name.trim() !== '' && state.name.trim() !== file.value.name,
)
const permsChanged = computed(() =>
  chmodEnabled.value && octalValid.value && state.octal.trim() !== initialOctal.value,
)
const canApply = computed(() => nameChanged.value || permsChanged.value)

function validate(s: Partial<typeof state>): FormError[] {
  const errors: FormError[] = []
  if (!s.name?.trim())
    errors.push({ name: 'name', message: t('modal.properties.errorEmpty') })
  if (chmodEnabled.value && s.octal?.trim() && !/^[0-7]{3,4}$/.test(s.octal.trim()))
    errors.push({ name: 'octal', message: t('modal.properties.errorInvalidMode') })
  return errors
}

async function onSubmit(event: FormSubmitEvent<typeof state>) {
  if (!file.value || !canApply.value || loading.value)
    return
  const dir = filesStore.currentPath.replace(/\/$/, '')
  const oldPath = `${dir}/${file.value.name}`
  loading.value = true
  apiError.value = null
  try {
    // chmod first (targets the current name), then rename
    const didChmod = permsChanged.value
    const didRename = nameChanged.value
    if (didChmod)
      await filesStore.chmod(oldPath, Number.parseInt(event.data.octal.trim(), 8))
    if (didRename)
      await filesStore.rename(oldPath, `${dir}/${event.data.name.trim()}`)
    if (didRename)
      notify.success(t('toast.renamed', { name: event.data.name.trim() }))
    if (didChmod)
      notify.success(t('toast.permissionsUpdated'))
    modalStore.close()
  }
  catch (e) {
    apiError.value = e instanceof Error ? e.message : t('error.operationFailed')
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
  <UModal v-model:open="open" :title="t('modal.properties.title')">
    <template #title>
      <UIcon name="i-lucide-info" class="size-5 text-muted" />
      {{ t('modal.properties.title') }}
    </template>

    <template #body>
      <UForm
        v-if="file"
        id="properties-form"
        :state="state"
        :validate="validate"
        class="space-y-6"
        @submit="onSubmit"
      >
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
          <UFormField name="name" class="flex-1 min-w-0">
            <UInput v-model="state.name" class="w-full" />
            <template #help>
              <span class="block text-xs text-dimmed truncate" :title="fullPath">{{ fullPath }}</span>
            </template>
          </UFormField>
        </div>

        <!-- Meta grid -->
        <div class="grid grid-cols-2 gap-y-3 gap-x-4 bg-elevated/40 p-3 rounded border border-default">
          <div>
            <span class="block label-caps text-muted mb-1">{{ t('modal.properties.size') }}</span>
            <span class="text-sm text-default">{{ formatSize(file.size) }}</span>
          </div>
          <div>
            <span class="block label-caps text-muted mb-1">{{ t('modal.properties.type') }}</span>
            <span class="text-sm text-default">{{ file.isDir ? t('modal.properties.typeDir') : t('modal.properties.typeFile') }}</span>
          </div>
          <div>
            <span class="block label-caps text-muted mb-1">{{ t('modal.properties.modified') }}</span>
            <span class="text-sm text-default">{{ formatDate(file.modified) }}</span>
          </div>
          <div>
            <span class="block label-caps text-muted mb-1">{{ t('modal.properties.permissions') }}</span>
            <span class="text-sm text-default">{{ file.mode || '--' }}</span>
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
              class="grid grid-cols-4 p-2 border-b border-default/50 last:border-b-0 items-center text-center text-sm"
            >
              <div class="text-left text-default pl-2">
                {{ t(`modal.properties.${group.key}`) }}
              </div>
              <div v-for="perm in PERM_BITS" :key="perm.key" class="flex justify-center">
                <UCheckbox
                  :model-value="permChecked(group.index, perm.bit)"
                  :aria-label="`${t(`modal.properties.${group.key}`)}: ${t(`modal.properties.${perm.key}`)}`"
                  @update:model-value="togglePerm(group.index, perm.bit)"
                />
              </div>
            </div>
          </div>
          <div class="flex items-start gap-3 mt-3 justify-end">
            <label for="properties-octal" class="label-caps text-muted mt-2">{{ t('modal.properties.numeric') }}</label>
            <UFormField name="octal" :ui="{ error: 'text-right' }">
              <UInput
                id="properties-octal"
                v-model="state.octal"
                maxlength="4"
                class="w-20"
                :ui="{ base: 'text-center text-primary' }"
              />
            </UFormField>
          </div>
        </div>

        <UAlert v-if="apiError" color="error" variant="soft" :description="apiError" />
      </UForm>
    </template>

    <template #footer="{ close }">
      <UButton color="neutral" variant="subtle" :label="t('modal.properties.cancel')" @click="close" />
      <UButton
        type="submit"
        form="properties-form"
        :disabled="!canApply"
        :loading="loading"
        :label="t('modal.properties.apply')"
      />
    </template>
  </UModal>
</template>
