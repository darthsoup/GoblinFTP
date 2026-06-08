<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'
import { ApiError } from '~/types/api'

const authStore = useAuthStore()
const { t } = useI18n()
const { appName, logoUrl, tagline } = useBranding()

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

// authStore.error survives across the connect lifecycle (and carries SSO
// failures surfaced before this form mounts); the local ref captures the
// manual-connect failure in onSubmit.
const displayError = computed(() => error.value ?? authStore.error)

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
    // On success `connected` flips; the layout's connected-watcher routes to
    // the workspace, which loads the directory listing.
    await authStore.connect({ ...form })
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
          <img v-if="logoUrl" :src="logoUrl" :alt="appName" class="size-8 object-contain">
          <UIcon v-else name="i-lucide-server" class="size-7 text-primary" />
          <h1 class="text-2xl font-bold tracking-tight text-highlighted">
            {{ appName }}
          </h1>
        </div>
        <p class="text-xs text-dimmed">
          {{ tagline }}
        </p>
      </div>

      <UAlert
        v-if="displayError"
        color="error"
        variant="soft"
        icon="i-lucide-triangle-alert"
        :description="displayError"
        class="mb-4"
      />

      <UForm :state="form" :validate="validate" class="space-y-4" @submit="onSubmit">
        <UFormField name="protocol" :label="t('login.protocol')">
          <USelect
            v-model="form.protocol"
            :items="protocolItems"
            class="w-full"
          />
        </UFormField>

        <div class="grid grid-cols-1 sm:grid-cols-[1fr_6rem] gap-3 items-start">
          <UFormField name="host" :label="t('login.host')">
            <UInput
              v-model="form.host"
              :placeholder="t('login.hostPlaceholder')"
              :disabled="hostLocked"
              class="w-full"
            />
          </UFormField>
          <UFormField name="port" :label="t('login.port')">
            <UInput
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
              :disabled="hostLocked"
              class="w-full"
            />
          </UFormField>
        </div>

        <UFormField name="username" :label="t('login.username')">
          <UInput
            v-model="form.username"
            :placeholder="t('login.usernamePlaceholder')"
            autocomplete="username"
            class="w-full"
          />
        </UFormField>

        <UFormField name="password" :label="t('login.password')">
          <UInput
            v-model="form.password"
            type="password"
            autocomplete="current-password"
            class="w-full"
          />
        </UFormField>

        <UFormField v-if="form.protocol === 'ftp' || form.protocol === 'ftps'" name="passive">
          <USwitch
            v-model="form.passive"
            size="sm"
            :label="t('login.passive')"
            :ui="{ label: 'text-xs text-muted' }"
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
