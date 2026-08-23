<template>
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
      <template v-if="showControls">
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
      <span v-else />
    </template>
  </UFooter>
</template>

<script setup lang="ts">
// Footer for the views without the app header: login screen and error page.
const authStore = useAuthStore()
const modalStore = useModalStore()
const { t } = useI18n()
const { appName, hideAttribution } = useBranding()
const { embedded } = useEmbed()

// Signed out with GFTP_LOGIN_FORM_DISABLED there is no way back into the app,
// so the preference controls stay out of reach until a session exists.
const showControls = computed(() => authStore.connected || !authStore.systemVars?.loginFormDisabled)
</script>
