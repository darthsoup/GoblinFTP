<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'
import { ApiError } from '~/types/api'

const authStore = useAuthStore()
const filesStore = useFilesStore()
const { t } = useI18n()

const form = reactive({
  protocol: 'ftp',
  host: '',
  port: 21,
  username: '',
  password: '',
  passive: true,
})

const error = ref<string | null>(null)
const loading = ref(false)

const protocolItems = computed(() =>
  authStore.allowedTypes.map(type => ({ label: type.toUpperCase(), value: type })),
)

const conn = computed(() => authStore.systemVars?.connection)
const hostLocked = computed(() => conn.value?.lockHost ?? false)

// Admin presets from settings.json: prefill host/port, default passive mode,
// and make sure the protocol is one the server allows. systemVars may arrive
// after mount, so apply reactively (without clobbering user input).
watch(conn, (c) => {
  if (!c)
    return
  if (!authStore.allowedTypes.includes(form.protocol))
    form.protocol = authStore.allowedTypes[0] ?? 'ftp'
  if (c.presetHost && !form.host)
    form.host = c.presetHost
  if (c.presetPort)
    form.port = c.presetPort
  form.passive = c.passiveMode
}, { immediate: true })

watch(() => form.protocol, (proto) => {
  form.port = conn.value?.presetPort ?? (proto === 'sftp' ? 22 : 21)
})

function validate(state: Partial<typeof form>): FormError[] {
  const errors: FormError[] = []
  if (!state.host?.trim())
    errors.push({ name: 'host', message: t('login.errorRequired') })
  if (!state.port || state.port < 1 || state.port > 65535)
    errors.push({ name: 'port', message: t('login.errorPort') })
  if (!state.username?.trim())
    errors.push({ name: 'username', message: t('login.errorRequired') })
  return errors
}

async function onSubmit(_event: FormSubmitEvent<typeof form>) {
  loading.value = true
  error.value = null
  try {
    await authStore.connect({ ...form })
    await filesStore.list(authStore.initialDirectory)
  }
  catch (e) {
    error.value = e instanceof ApiError ? e.message : t('error.connectionFailed')
  }
  finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="flex flex-1 items-center justify-center p-4">
    <div class="w-full max-w-md bg-elevated border border-default rounded-lg p-8 shadow-xl">
      <div class="flex flex-col items-center gap-1.5 mb-8 select-none">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-server" class="size-7 text-primary" />
          <h1 class="text-2xl font-bold tracking-tight text-highlighted">
            GoblinFTP
          </h1>
        </div>
        <p class="font-mono text-xs text-dimmed">
          {{ t('login.tagline') }}
        </p>
      </div>

      <UAlert
        v-if="error"
        color="error"
        variant="soft"
        icon="i-lucide-triangle-alert"
        :description="error"
        class="mb-4"
      />

      <UForm :state="form" :validate="validate" class="space-y-4" @submit="onSubmit">
        <UFormField name="protocol" :label="t('login.protocol')">
          <USelect
            v-model="form.protocol"
            :items="protocolItems"
            class="w-full font-mono"
          />
        </UFormField>

        <div class="grid grid-cols-[1fr_6rem] gap-3 items-start">
          <UFormField name="host" :label="t('login.host')">
            <UInput
              v-model="form.host"
              :placeholder="t('login.hostPlaceholder')"
              :disabled="hostLocked"
              class="w-full font-mono"
            />
          </UFormField>
          <UFormField name="port" :label="t('login.port')">
            <UInput
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
              :disabled="hostLocked"
              class="w-full font-mono"
            />
          </UFormField>
        </div>

        <UFormField name="username" :label="t('login.username')">
          <UInput
            v-model="form.username"
            :placeholder="t('login.usernamePlaceholder')"
            autocomplete="username"
            class="w-full font-mono"
          />
        </UFormField>

        <UFormField name="password" :label="t('login.password')">
          <UInput
            v-model="form.password"
            type="password"
            autocomplete="current-password"
            class="w-full font-mono"
          />
        </UFormField>

        <UFormField v-if="form.protocol === 'ftp'" name="passive">
          <USwitch
            v-model="form.passive"
            size="sm"
            :label="t('login.passive')"
            :ui="{ label: 'font-mono text-xs text-muted' }"
          />
        </UFormField>

        <UButton
          type="submit"
          icon="i-lucide-plug"
          :loading="loading"
          class="mt-6 justify-center"
          block
        >
          {{ t('login.connect') }}
        </UButton>
      </UForm>
    </div>
  </div>
</template>
