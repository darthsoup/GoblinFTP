export default defineAppConfig({
  ui: {
    // white panel, comfortable lg items
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
        // 14px / semibold
        label: 'text-sm font-semibold',
        // messages: 12px / medium; 4px gap to the control
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
        // elevated bg, no divider lines (whitespace-separated), generous padding
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
