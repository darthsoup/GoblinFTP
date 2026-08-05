<script setup lang="ts">
// The unauthenticated view: centered login form (or a boot spinner while an SSO
// auto-connect resolves) plus a footer with pre-connect controls. pages/login.vue
// owns `booting` and the SSO landing; this is purely presentational.
defineProps<{ booting: boolean }>()

const authStore = useAuthStore()
const modalStore = useModalStore()
const { t } = useI18n()
const { appName, hideAttribution } = useBranding()
const { embedded } = useEmbed()

// GFTP_LOGIN_FORM_DISABLED was published in systemVars and typed on the client
// but read by no component, so it did nothing. It matters most for an embed:
// when a panel session ends, routing to /login otherwise renders a credential
// form the panel user has no credentials for.
const loginFormDisabled = computed(() => authStore.systemVars?.loginFormDisabled ?? false)
</script>

<template>
  <UMain class="flex-1 min-h-0 flex flex-col">
    <UContainer class="flex flex-1 flex-col items-center justify-center py-10">
      <div v-if="booting" class="flex items-center justify-center">
        <UIcon name="i-lucide-loader-circle" class="size-10 animate-spin text-primary" />
      </div>
      <div v-else-if="loginFormDisabled" class="max-w-md text-center">
        <UIcon name="i-lucide-plug-zap" class="size-10 text-dimmed mb-4" />
        <h2 class="text-lg font-semibold text-highlighted mb-2">
          {{ t('login.sessionEndedTitle') }}
        </h2>
        <p class="text-sm text-muted">
          {{ t('login.sessionEndedMessage') }}
        </p>
      </div>
      <LoginForm v-else />
    </UContainer>
  </UMain>

  <UFooter
    :ui="{
      root: 'shrink-0 border-t border-default bg-elevated/50',
      container: 'px-4 py-0 h-14 flex items-center justify-between gap-3',
      left: 'mt-0 gap-x-1.5',
      right: 'mt-0 gap-x-1 justify-end',
    }"
  >
    <template #left>
      <span v-if="!hideAttribution && !embedded" class="text-sm text-dimmed select-none">
        {{ appName }} {{ authStore.systemVars?.version ?? '' }}
      </span>
      <span v-else />
    </template>

    <template #right>
      <LanguageSelect variant="ghost" size="lg" />
      <ColorModeButton />
      <UTooltip :text="t('header.settings')">
        <UButton
          color="neutral"
          variant="ghost"
          icon="i-lucide-settings"
          :aria-label="t('header.settings')"
          @click="modalStore.open('settings')"
        />
      </UTooltip>
    </template>
  </UFooter>

  <HostKeyVerifyModal />
</template>
