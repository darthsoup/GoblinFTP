<script setup lang="ts">
const modalStore = useModalStore()
const { t } = useI18n()

const open = computed({
  get: () => modalStore.active === 'shortcuts',
  set: (v: boolean) => {
    if (!v)
      modalStore.close()
  },
})

// `keys` are UKbd values: 'meta'/'alt'/'arrowup'/'escape' alias to symbols
// (⌘/Ctrl, ⌥/Alt, ↑, Esc); literals like 'F2'/'A'/'Del'/'?' render as-is.
const groups = computed(() => [
  {
    title: t('shortcuts.groupActions'),
    items: [
      { keys: ['F2'], label: t('shortcuts.rename') },
      { keys: ['Del'], label: t('shortcuts.delete') },
      { keys: ['meta', 'C'], label: t('shortcuts.copy') },
      { keys: ['meta', 'X'], label: t('shortcuts.cut') },
      { keys: ['meta', 'V'], label: t('shortcuts.paste') },
      { keys: ['escape'], label: t('shortcuts.clear') },
    ],
  },
  {
    title: t('shortcuts.groupSelect'),
    items: [
      { keys: ['meta', 'A'], label: t('shortcuts.selectAll') },
    ],
  },
  {
    title: t('shortcuts.groupNav'),
    items: [
      { keys: ['alt', 'arrowup'], label: t('shortcuts.up') },
      { keys: ['alt', 'arrowleft'], label: t('shortcuts.back') },
      { keys: ['alt', 'arrowright'], label: t('shortcuts.forward') },
    ],
  },
])
</script>

<template>
  <UModal v-model:open="open" :title="t('shortcuts.title')">
    <template #title>
      <UIcon name="i-lucide-keyboard" class="size-5 text-muted" />
      {{ t('shortcuts.title') }}
    </template>

    <template #body>
      <div class="space-y-5">
        <div v-for="group in groups" :key="group.title">
          <h3 class="label-caps text-muted border-b border-default pb-1 mb-2">
            {{ group.title }}
          </h3>
          <ul class="space-y-1.5">
            <li
              v-for="item in group.items"
              :key="item.label"
              class="flex items-center justify-between gap-4"
            >
              <span class="text-sm text-default">{{ item.label }}</span>
              <span class="flex items-center gap-1">
                <UKbd v-for="k in item.keys" :key="k" :value="k" />
              </span>
            </li>
          </ul>
        </div>
      </div>
    </template>
  </UModal>
</template>
