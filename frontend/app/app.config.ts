export default defineAppConfig({
  ui: {
    colors: {
      primary: 'goblin',
      neutral: 'neutral',
    },
    // Tech-Dark modal anatomy: compact header/footer strips on elevated surface,
    // divide-y from the modal theme draws the separators.
    modal: {
      slots: {
        content: 'min-w-96',
        header: 'px-4 py-3 sm:px-4 min-h-0 bg-elevated/60',
        body: 'p-5 sm:p-5',
        footer: 'px-4 py-3 sm:px-4 bg-elevated/60 justify-end',
        title: 'text-base flex items-center gap-2',
        close: 'top-2.5 end-3',
      },
    },
    // Popover surface: charcoal tier in dark, white card with slate ring in light.
    contextMenu: {
      slots: {
        content: 'min-w-48 bg-(--gftp-popover) ring-(--gftp-popover-ring)',
      },
    },
    // Path segments: muted with primary hover; current segment bold primary.
    breadcrumb: {
      variants: {
        active: {
          true: { link: 'font-bold text-primary' },
          false: { link: 'font-medium text-muted hover:text-primary' },
        },
      },
    },
    // Field labels follow the label-caps convention everywhere.
    formField: {
      slots: {
        label: 'label-caps font-bold text-muted',
        error: 'mt-1.5 text-xs',
      },
    },
  },
})
