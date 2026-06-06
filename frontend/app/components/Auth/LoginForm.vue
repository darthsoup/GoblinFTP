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

watch(() => form.protocol, (proto) => {
  form.port = proto === 'sftp' ? 22 : 21
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
              class="w-full font-mono"
            />
          </UFormField>
          <UFormField name="port" :label="t('login.port')">
            <UInput
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
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
