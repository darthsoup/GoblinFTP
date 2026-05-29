<script setup lang="ts">
const filesStore = useFilesStore()
const { t } = useI18n()
</script>

<template>
  <nav class="flex items-center gap-1 px-4 py-2 bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 text-sm overflow-x-auto">
    <!-- Root -->
    <button
      class="flex items-center gap-1 hover:text-primary-500 transition-colors shrink-0"
      @click="filesStore.navigate('/')"
    >
      <UIcon name="i-heroicons-home" class="w-4 h-4" />
      <span class="sr-only">{{ t('breadcrumb.root') }}</span>
    </button>

    <template v-for="(seg, i) in filesStore.pathSegments" :key="seg.path">
      <UIcon name="i-heroicons-chevron-right" class="w-3 h-3 text-gray-400 shrink-0" />
      <button
        class="hover:text-primary-500 transition-colors truncate max-w-[12rem]"
        :class="{ 'font-medium text-gray-900 dark:text-gray-100': i === filesStore.pathSegments.length - 1 }"
        @click="filesStore.navigate(seg.path)"
      >
        {{ seg.label }}
      </button>
    </template>
  </nav>
</template>
