export default defineAppConfig({
  ui: {
    icons: {
      arrowDown: 'i-ph-arrow-down',
      arrowLeft: 'i-ph-arrow-left',
      arrowRight: 'i-ph-arrow-right',
      arrowUp: 'i-ph-arrow-up',
      caution: 'i-ph-warning-circle',
      check: 'i-ph-check',
      chevronDoubleLeft: 'i-ph-caret-double-left',
      chevronDoubleRight: 'i-ph-caret-double-right',
      chevronDown: 'i-ph-caret-down',
      chevronLeft: 'i-ph-caret-left',
      chevronRight: 'i-ph-caret-right',
      chevronUp: 'i-ph-caret-up',
      close: 'i-ph-x',
      copy: 'i-ph-copy',
      copyCheck: 'i-ph-checks',
      dark: 'i-ph-moon',
      drag: 'i-ph-dots-six-vertical',
      ellipsis: 'i-ph-dots-three',
      error: 'i-ph-x-circle',
      external: 'i-ph-arrow-up-right',
      eye: 'i-ph-eye',
      eyeOff: 'i-ph-eye-slash',
      file: 'i-ph-file',
      folder: 'i-ph-folder',
      folderOpen: 'i-ph-folder-open',
      hash: 'i-ph-hash',
      info: 'i-ph-info',
      light: 'i-ph-sun',
      loading: 'i-ph-spinner-gap',
      menu: 'i-ph-list',
      minus: 'i-ph-minus',
      panelClose: 'i-ph-sidebar-simple',
      panelOpen: 'i-ph-sidebar',
      plus: 'i-ph-plus',
      reload: 'i-ph-arrow-counter-clockwise',
      search: 'i-ph-magnifying-glass',
      stop: 'i-ph-square',
      star: 'i-ph-star',
      success: 'i-ph-check-circle',
      system: 'i-ph-monitor',
      tip: 'i-ph-lightbulb',
      upload: 'i-ph-upload-simple',
      warning: 'i-ph-warning',
    },

    header: {
      slots: {
        container: 'lg:px-4',
      },
    },

    breadcrumb: {
      variants: {
        active: {
          true: {
            link: 'font-semibold',
          },
          false: {
            link: 'text-muted font-medium',
          },
        },
      },
    },

    contextMenu: {
      slots: { content: 'min-w-48 bg-elevated' },
      defaultVariants: { size: 'lg' },
    },

    dropdownMenu: {
      slots: { content: 'min-w-48 bg-elevated' },
      defaultVariants: { size: 'lg' },
    },

    // inverted bubble (no ring/shadow); the v4 default is a light surface
    tooltip: {
      slots: {
        content:
          'bg-inverted text-inverted ring-0 shadow-none h-auto p-2 max-w-[200px] text-left font-medium leading-normal',
        // match the inverted bubble; stroke hides the (now-absent) ring seam
        arrow: 'fill-inverted stroke-inverted',
      },
    },

    formField: {
      slots: {
        label: 'text-sm font-semibold',
        error: 'mt-1 text-xs font-medium text-error',
        help: 'mt-1 text-xs font-medium text-muted',
        description: 'text-xs font-medium text-muted',
      },
    },

    // form-surface fill + 4px radius (reference .gv-checkbox); border + checked green unchanged
    checkbox: {
      slots: {
        base: 'rounded-[4px] bg-[var(--gftp-input-bg)]',
      },
    },

    modal: {
      slots: {
        // no divider lines: sections are separated by whitespace instead
        content: 'bg-elevated divide-y-0',
        header: 'px-6 sm:px-8 pt-6 sm:pt-8 pb-0 min-h-0',
        body: 'px-6 sm:px-8 py-5 sm:py-6',
        footer: 'px-6 sm:px-8 pt-0 pb-6 sm:pb-8',
        // flex so the leading icon centers against the larger title text
        title: 'flex items-center gap-2 text-lg font-bold',
      },
    },

    button: {
      slots: { base: 'font-semibold' },
      defaultVariants: { size: 'lg' },
      variants: {
        size: {
          // condensed: 12px / ~30px, for dense surfaces (file toolbar)
          sm: { base: 'px-4 py-1.5 text-xs gap-1.5' },
          lg: { base: 'min-h-12 px-4 py-2 text-sm gap-2' },
        },
      },
      compoundVariants: [
        // keep icon-only (square) lg buttons square at the 48px height
        { size: 'lg', square: true },
      ],
    },

    input: {
      defaultVariants: { size: 'lg' },

      slots: {
        base: `font-medium placeholder:text-[var(--gftp-input-placeholder)]`,
      },

      variants: {
        variant: {
          outline: 'bg-[var(--gftp-input-bg)]',
        },
        size: {
          // condensed: 16px H-padding / ~30px, mirrors the button condensed size
          sm: { base: 'px-4 py-1.5 text-xs gap-1.5' },
          lg: { base: 'min-h-12 px-4 py-2 text-sm gap-2' },
        },
      },

      compoundVariants: [{ color: 'primary', variant: 'outline', class: 'focus-visible:ring-1' }],
    },

    inputNumber: {
      defaultVariants: { size: 'lg' },

      slots: {
        base: `font-medium placeholder:text-[var(--gftp-input-placeholder)]`,
      },

      variants: {
        variant: {
          outline: 'bg-[var(--gftp-input-bg)]',
        },
        size: {
          sm: { base: 'px-4 py-1.5 text-xs gap-1.5' },
          lg: { base: 'min-h-12 px-4 py-2 text-sm gap-2' },
        },
      },

      compoundVariants: [{ color: 'primary', variant: 'outline', class: 'focus-visible:ring-1' }],
    },

    inputMenu: {
      defaultVariants: { size: 'lg' },

      slots: {
        base: `font-medium placeholder:text-[var(--gftp-input-placeholder)]`,
      },

      variants: {
        variant: {
          outline: 'bg-[var(--gftp-input-bg)]',
        },
        size: {
          sm: { base: 'px-4 py-1.5 text-xs gap-1.5' },
          lg: { base: 'min-h-12 px-4 py-2 text-sm gap-2' },
        },
      },

      compoundVariants: [{ color: 'primary', variant: 'outline', class: 'focus-visible:ring-1' }],
    },

    select: {
      defaultVariants: { size: 'lg' },
      // select's placeholder is a span slot, not an <input> ::placeholder
      slots: {
        placeholder: 'text-[var(--gftp-input-placeholder)]',
      },
      variants: {
        variant: {
          outline: 'bg-[var(--gftp-input-bg)]',
        },
        size: {
          sm: { base: 'px-4 py-1.5 text-xs gap-1.5' },
          lg: { base: 'min-h-12 px-4 py-2 text-sm gap-2' },
        },
      },
      compoundVariants: [{ color: 'primary', variant: 'outline', class: 'focus-visible:ring-1' }],
    },

    selectMenu: {
      defaultVariants: { size: 'lg' },
      slots: {
        placeholder: 'text-[var(--gftp-input-placeholder)]',
      },
      variants: {
        variant: {
          outline: 'bg-[var(--gftp-input-bg)]',
        },
        size: {
          sm: { base: 'px-4 py-1.5 text-xs gap-1.5' },
          lg: { base: 'min-h-12 px-4 py-2 text-sm gap-2' },
        },
      },
      compoundVariants: [{ color: 'primary', variant: 'outline', class: 'focus-visible:ring-1' }],
    },

    textarea: {
      defaultVariants: { size: 'lg' },
      slots: {
        base: 'placeholder:text-[var(--gftp-input-placeholder)]',
      },
      variants: {
        variant: {
          outline: 'bg-[var(--gftp-input-bg)]',
        },
        size: {
          sm: { base: 'px-4 py-1.5 text-xs gap-1.5' },
          lg: { base: 'min-h-12 px-4 py-2 text-sm gap-2' },
        },
      },
      compoundVariants: [{ color: 'primary', variant: 'outline', class: 'focus-visible:ring-1' }],
    },
  },
})
