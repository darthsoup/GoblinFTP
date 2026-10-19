<template>
  <section class="flex h-full min-h-0 flex-col" :aria-label="t('tree.title')">
    <div class="flex h-12 shrink-0 items-center justify-between gap-2 border-b border-default px-3">
      <span class="label-caps text-muted">{{ t('tree.title') }}</span>
      <UButton
        size="sm"
        color="neutral"
        variant="ghost"
        icon="i-ph-arrows-clockwise"
        :aria-label="t('tree.refresh')"
        @click="directories.invalidate(['/'])"
      />
    </div>
    <UTree
      ref="tree"
      :model-value="selected"
      :expanded="directories.expanded.map(path => `directory:${path}`)"
      :items="items"
      :get-key="item => item.key"
      :virtualize="{ estimateSize: 48 }"
      size="lg"
      :aria-label="t('tree.title')"
      class="min-h-0 flex-1 overflow-auto p-2"
      :ui="{ link: 'min-h-12 w-full', linkLabel: 'min-w-0' }"
      @update:expanded="directories.expanded = $event.map(key => key.slice('directory:'.length))"
      @select="onSelect"
      @toggle="onToggle"
    >
      <template #item="{ item, expanded }">
        <span
          class="flex size-6 shrink-0 items-center justify-center rounded hover:bg-accented"
          :data-disclosure="item.kind === 'directory' && item.children ? item.path : undefined"
          aria-hidden="true"
          @click.stop="toggle(item)"
        >
          <UIcon v-if="item.kind === 'directory' && item.children" :name="expanded ? 'i-ph-caret-down' : 'i-ph-caret-right'" class="size-4" />
        </span>
        <UIcon
          :name="item.kind === 'error' ? 'i-ph-arrow-counter-clockwise' : item.path === '/' ? 'i-ph-hard-drives-duotone' : expanded ? 'i-ph-folder-open-duotone' : 'i-ph-folder-duotone'"
          class="size-5 shrink-0"
          :class="item.kind === 'error' ? 'text-error' : 'text-primary'"
        />
        <span class="min-w-0 flex-1 truncate text-left" :title="item.kind === 'error' ? item.label : item.path">
          {{ item.label }}
        </span>
        <UIcon v-if="directories.listings[item.path]?.loading" name="i-ph-spinner-gap" class="size-4 shrink-0 animate-spin text-muted" :aria-label="t('files.loading')" />
        <UIcon v-else-if="item.kind === 'directory' && directories.listings[item.path]?.errorCode" name="i-ph-warning" class="size-4 shrink-0 text-error" :aria-label="errorLabel(item.path)" />
      </template>
    </UTree>
  </section>
</template>

<script setup lang="ts">
import type { TreeItem } from '@nuxt/ui'
import { ancestorPaths, parentPath } from '~/utils/remotePath'

interface DirectoryNode extends TreeItem {
  key: string
  path: string
  kind: 'directory' | 'error'
  children?: DirectoryNode[]
}

const emit = defineEmits<{ navigated: [] }>()
const directories = useDirectoriesStore()
const files = useFilesStore()
const settings = useSettingsStore()
const auth = useAuthStore()
const { t } = useI18n()
const { localizeError } = useErrorMessage()
const tree = useTemplateRef('tree')
let revealSequence = 0

function errorLabel(path: string): string {
  return localizeError(directories.listings[path]?.errorCode ?? '', t('tree.loadFailed'))
}

const items = computed<DirectoryNode[]>(() => {
  const active = ancestorPaths(files.currentPath)
  function node(path: string): DirectoryNode {
    const state = directories.listings[path]
    const childPaths = new Set((state?.files ?? [])
      .filter(file => file.isDir && file.name && file.name !== '.' && file.name !== '..' && !file.name.includes('/'))
      .map(file => `${path === '/' ? '' : path}/${file.name}`)
      .filter(child => settings.showDotfiles || !child.split('/').pop()!.startsWith('.') || active.includes(child)))
    const knownChild = active.find(child => child !== '/' && parentPath(child) === path)
    if (knownChild)
      childPaths.add(knownChild)
    const expandable = state?.files == null || state.dirty || state.errorCode || childPaths.size > 0
    const children: DirectoryNode[] = []
    if (directories.expanded.includes(path)) {
      if (state?.errorCode)
        children.push({ key: `error:${path}`, path, kind: 'error', label: `${t('files.retry')}: ${errorLabel(path)}` })
      children.push(...[...childPaths].sort((a, b) => a.localeCompare(b)).map(node))
    }
    return {
      key: `directory:${path}`,
      path,
      kind: 'directory',
      label: path === '/' ? '/' : path.split('/').pop()!,
      ...(expandable ? { children } : {}),
    }
  }
  return [node('/')]
})

const selected = computed<DirectoryNode>(() => ({
  key: `directory:${files.currentPath}`,
  path: files.currentPath,
  kind: 'directory',
}))

const rows = computed(() => {
  function flatten(nodes: DirectoryNode[]): DirectoryNode[] {
    return nodes.flatMap(node => [node, ...flatten(node.children ?? [])])
  }
  return flatten(items.value)
})

function toggle(item: DirectoryNode) {
  if (item.kind !== 'directory' || !item.children)
    return
  directories.expanded = directories.expanded.includes(item.path)
    ? directories.expanded.filter(path => path !== item.path)
    : [...directories.expanded, item.path]
}

async function onSelect(event: Event, item: DirectoryNode) {
  event.preventDefault()
  if (item.kind === 'error') {
    await directories.loadTree(item.path).catch(() => {})
    return
  }
  if (await files.navigate(item.path))
    emit('navigated')
}

function onToggle(event: Event) {
  if ((event as CustomEvent<{ originalEvent: Event }>).detail.originalEvent.type !== 'keydown')
    event.preventDefault()
}

function visible(path: string): boolean {
  return rows.value.some(node => node.kind === 'directory' && node.path === path)
}

async function refreshExpanded() {
  const sequence = revealSequence
  for (const path of directories.expanded) {
    if (sequence !== revealSequence || !auth.connected || auth.sessionLost)
      return
    const state = directories.listings[path]
    if (directories.expanded.includes(path) && visible(path) && (!state?.files || state.dirty) && !state?.errorCode)
      await directories.loadTree(path).catch(() => {})
  }
}

watch(() => files.currentPath, async (path) => {
  const sequence = ++revealSequence
  const ancestors = ancestorPaths(path)
  directories.expanded = [...new Set([...directories.expanded, ...ancestors])]
  for (const ancestor of ancestors) {
    if (sequence !== revealSequence || !auth.connected || auth.sessionLost)
      return
    await directories.loadTree(ancestor).catch(() => {})
  }
  await nextTick()
  if (sequence !== revealSequence)
    return
  const root = tree.value?.$el as HTMLElement | undefined
  const top = rows.value.findIndex(node => node.kind === 'directory' && node.path === path) * 48
  if (root && (top < root.scrollTop || top + 48 > root.scrollTop + root.clientHeight))
    root.scrollTop = Math.max(0, top - root.clientHeight / 2 + 24)
}, { immediate: true })

watch(() => directories.expanded, () => {
  void refreshExpanded()
}, { deep: true })
watch(() => directories.invalidation, useDebounceFn(refreshExpanded, 200))
watch(() => directories.expanded.map((path) => {
  const state = directories.listings[path]
  return state?.dirty && !state.loading && !state.errorCode ? `${path}:${state.revision}` : ''
}), () => {
  void refreshExpanded()
})
onScopeDispose(() => {
  revealSequence++
})
</script>
