<script setup lang="ts">
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

watch(() => form.protocol, (proto) => {
  form.port = proto === 'sftp' ? 22 : 21
})

async function handleSubmit() {
  if (!form.host || !form.username)
    return
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

      <form class="space-y-4" @submit.prevent="handleSubmit">
        <!-- Protocol -->
        <div>
          <label class="block label-caps text-muted mb-1.5">{{ t('login.protocol') }}</label>
          <select
            v-model="form.protocol"
            class="w-full rounded-md bg-default border border-accented px-3 py-2 text-sm font-mono text-default focus:outline-none focus:border-primary focus:ring-1 focus:ring-primary transition-colors"
          >
            <option v-for="type in authStore.allowedTypes" :key="type" :value="type">
              {{ type.toUpperCase() }}
            </option>
          </select>
        </div>

        <!-- Host + Port -->
        <div class="grid grid-cols-[1fr_6rem] gap-3">
          <div>
            <label class="block label-caps text-muted mb-1.5">{{ t('login.host') }}</label>
            <UInput
              v-model="form.host"
              :placeholder="t('login.hostPlaceholder')"
              required
              class="w-full font-mono"
            />
          </div>
          <div>
            <label class="block label-caps text-muted mb-1.5">{{ t('login.port') }}</label>
            <UInput
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
              required
              class="w-full font-mono"
            />
          </div>
        </div>

        <!-- Username -->
        <div>
          <label class="block label-caps text-muted mb-1.5">{{ t('login.username') }}</label>
          <UInput
            v-model="form.username"
            :placeholder="t('login.usernamePlaceholder')"
            autocomplete="username"
            required
            class="w-full font-mono"
          />
        </div>

        <!-- Password -->
        <div>
          <label class="block label-caps text-muted mb-1.5">{{ t('login.password') }}</label>
          <UInput
            v-model="form.password"
            type="password"
            autocomplete="current-password"
            class="w-full font-mono"
          />
        </div>

        <UButton
          type="submit"
          icon="i-lucide-plug"
          :loading="loading"
          class="w-full justify-center mt-6"
          block
        >
          {{ t('login.connect') }}
        </UButton>
      </form>
    </div>
  </div>
</template>
