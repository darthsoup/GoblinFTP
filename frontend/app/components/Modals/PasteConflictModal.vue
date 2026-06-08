<script setup lang="ts">
// Asks how to resolve name collisions during a paste. Driven by
// modalStore.pasteConflict() (a promise the files store awaits). Closing via
// backdrop/Esc resolves as 'cancel'.
const modalStore = useModalStore()
const { t } = useI18n()

const conflicts = computed(() => modalStore.pasteConflicts)
</script>

<template>
  <UModal
    :open="modalStore.active === 'pasteConflict'"
    :title="t('modal.pasteConflict.title')"
    @update:open="(v: boolean) => { if (!v) modalStore.resolvePaste('cancel') }"
  >
    <template #body>
      <p class="text-sm text-muted mb-3">
        {{ t('modal.pasteConflict.message') }}
      </p>
      <ul class="max-h-48 overflow-auto rounded border border-default bg-muted/40 px-3 py-2 space-y-1">
        <li
          v-for="name in conflicts"
          :key="name"
          class="font-mono text-sm text-highlighted truncate"
        >
          {{ name }}
        </li>
      </ul>
    </template>

    <template #footer>
      <div class="flex w-full items-center justify-end gap-2">
        <UButton color="neutral" variant="ghost" :label="t('modal.pasteConflict.cancel')" @click="modalStore.resolvePaste('cancel')" />
        <UButton color="error" variant="soft" :label="t('modal.pasteConflict.overwrite')" @click="modalStore.resolvePaste('overwrite')" />
        <UButton color="primary" :label="t('modal.pasteConflict.append')" @click="modalStore.resolvePaste('append')" />
      </div>
    </template>
  </UModal>
</template>
