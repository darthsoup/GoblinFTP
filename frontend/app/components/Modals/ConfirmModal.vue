<script setup lang="ts">
// Generic confirm dialog driven by modalStore.confirm() (a promise). Labels and
// copy come from the caller, so this component stays i18n-agnostic.
const modalStore = useModalStore()
const opts = computed(() => modalStore.confirmOptions)
</script>

<template>
  <UModal
    :open="!!opts"
    :title="opts?.title"
    @update:open="(v: boolean) => { if (!v) modalStore.resolveConfirm('cancel') }"
  >
    <template #body>
      <p class="text-sm text-muted">
        {{ opts?.message }}
      </p>
    </template>

    <template #footer>
      <div class="flex w-full items-center justify-end gap-2">
        <UButton color="neutral" variant="ghost" :label="opts?.cancelLabel" @click="modalStore.resolveConfirm('cancel')" />
        <UButton v-if="opts?.saveLabel" color="primary" variant="subtle" :label="opts.saveLabel" @click="modalStore.resolveConfirm('save')" />
        <UButton :color="opts?.confirmColor ?? 'primary'" :label="opts?.confirmLabel" @click="modalStore.resolveConfirm('confirm')" />
      </div>
    </template>
  </UModal>
</template>
