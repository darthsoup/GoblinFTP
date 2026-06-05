---
name: Goblin Tech-Dark
colors:
  surface: '#10141a'
  surface-dim: '#10141a'
  surface-bright: '#353940'
  surface-container-lowest: '#0a0e14'
  surface-container-low: '#181c22'
  surface-container: '#1c2026'
  surface-container-high: '#262a31'
  surface-container-highest: '#31353c'
  on-surface: '#dfe2eb'
  on-surface-variant: '#b9ccb2'
  inverse-surface: '#dfe2eb'
  inverse-on-surface: '#2d3137'
  outline: '#84967e'
  outline-variant: '#3b4b37'
  surface-tint: '#00e639'
  primary: '#ebffe2'
  on-primary: '#003907'
  primary-container: '#00ff41'
  on-primary-container: '#007117'
  inverse-primary: '#006e16'
  secondary: '#67df70'
  on-secondary: '#00390d'
  secondary-container: '#27a640'
  on-secondary-container: '#00320a'
  tertiary: '#f9f9ff'
  on-tertiary: '#002f65'
  tertiary-container: '#ceddff'
  on-tertiary-container: '#005fbe'
  error: '#ffb4ab'
  on-error: '#690005'
  error-container: '#93000a'
  on-error-container: '#ffdad6'
  primary-fixed: '#72ff70'
  primary-fixed-dim: '#00e639'
  on-primary-fixed: '#002203'
  on-primary-fixed-variant: '#00530e'
  secondary-fixed: '#83fc89'
  secondary-fixed-dim: '#67df70'
  on-secondary-fixed: '#002105'
  on-secondary-fixed-variant: '#005317'
  tertiary-fixed: '#d7e3ff'
  tertiary-fixed-dim: '#aac7ff'
  on-tertiary-fixed: '#001b3e'
  on-tertiary-fixed-variant: '#00458e'
  background: '#10141a'
  on-background: '#dfe2eb'
  surface-variant: '#31353c'
typography:
  headline-lg:
    fontFamily: Inter
    fontSize: 32px
    fontWeight: '600'
    lineHeight: '1.2'
  headline-md:
    fontFamily: Inter
    fontSize: 24px
    fontWeight: '600'
    lineHeight: '1.3'
  body-lg:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '400'
    lineHeight: '1.5'
  body-md:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '400'
    lineHeight: '1.5'
  code-md:
    fontFamily: JetBrains Mono
    fontSize: 14px
    fontWeight: '400'
    lineHeight: '1.6'
  code-sm:
    fontFamily: JetBrains Mono
    fontSize: 12px
    fontWeight: '400'
    lineHeight: '1.6'
  label-caps:
    fontFamily: Inter
    fontSize: 11px
    fontWeight: '700'
    lineHeight: '1.4'
    letterSpacing: 0.05em
rounded:
  sm: 0.125rem
  DEFAULT: 0.25rem
  md: 0.375rem
  lg: 0.5rem
  xl: 0.75rem
  full: 9999px
spacing:
  base: 4px
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 32px
  gutter: 16px
  margin-mobile: 16px
  margin-desktop: 24px
---

## Brand & Style

This design system is built for a high-performance, developer-centric FTP client. The brand personality is technical, precise, and authoritative, evoking the feeling of a sophisticated terminal environment translated into a modern web interface. 

The aesthetic leverages **Minimalism** with a **Corporate/Modern** backbone, utilizing deep "hacker" tones and high-contrast accents to ensure clarity during long-duration technical tasks. It prioritizes information density and speed of recognition, using "Goblin Green" as a functional beacon for status and primary interactions. The overall mood is "Professional Dark Mode"—stable, efficient, and trustworthy.

## Colors

The palette is anchored in a deep navy-charcoal spectrum to minimize eye strain and maximize the pop of functional colors. 

- **Primary (Goblin Green):** Used exclusively for successful states, active connections, and primary action buttons.
- **Surface Tiers:** Backgrounds use the deepest navy (#0D1117), while interactive panels and cards use a slightly lighter charcoal (#161B22) to create depth.
- **Functional Accents:** Muted teals and blues are reserved for non-critical information like folder icons or secondary navigation links.
- **Status Signal:** High-contrast logic applies to error states (Red), transferring states (Blue), and idle states (Gray).

## Typography

The typography system uses a dual-font strategy. **Inter** provides a clean, neutral canvas for the general UI, ensuring high legibility in menus and settings. **JetBrains Mono** is the functional workhorse, applied to all data-heavy strings including file paths, permissions (e.g., `drwxr-xr-x`), file sizes, and timestamps.

On mobile devices, headlines scale down by 20% to conserve vertical space, while monospaced data maintains a minimum of 12px for technical accuracy.

## Layout & Spacing

The design system employs a **Fluid Grid** model with high density. The primary workspace (the file explorer) uses a sidebar/main-content split. 

- **Density:** Padding is kept tight (8px-12px) in list items to maximize the number of visible files.
- **Grid:** A 12-column system is used for dashboard views, while the file list operates on a flexible flexbox/grid hybrid where columns (Name, Size, Modified) have minimum widths but expand to fill horizontal space.
- **Breakpoints:** 
    - Desktop (>1024px): Multi-pane view (Folders | Files | Queue).
    - Tablet (768px - 1024px): Foldable sidebar, stacked queue.
    - Mobile (<768px): Single pane view with bottom-sheet navigation for file actions.

## Elevation & Depth

Depth is established through **Tonal Layers** and **Low-contrast Outlines** rather than heavy shadows. 

- **Level 0 (Background):** #0D1117 - The base "canvas."
- **Level 1 (Panels):** #161B22 - Used for the main sidebar and file list container.
- **Level 2 (Popovers/Modals):** #21262D - Higher contrast for items "closer" to the user, paired with a subtle 1px border (#30363D).
- **Interactive States:** Hover states on list items are indicated by a subtle background shift to #1C2128, creating a "lit" effect without needing physical elevation.

## Shapes

The shape language is **Soft (0.25rem)**. This maintains a precise, engineering-focused look while avoiding the harshness of 0px corners.

- **Buttons & Inputs:** 4px (0.25rem) radius.
- **Cards & Modals:** 8px (0.5rem) radius for a more structured feel.
- **Status Pills:** Fully rounded (pill-shaped) to distinguish them from interactive buttons.

## Components

- **Buttons:** Primary buttons use a "Goblin Green" background with black text for maximum contrast. Secondary buttons use a ghost style with a subtle gray border.
- **File List Items:** Rows must have a fixed height (40px or 48px). Use alternating row highlights ("zebra striping") very subtly for long list scanning.
- **Status Indicators:** A small circular dot (8px) paired with text. Green = Connected, Amber = Connecting/Warning, Red = Error, Blue = Transferring.
- **Input Fields:** Monospaced text entry for paths and hostnames. Borders should brighten to the primary color only on focus.
- **Scrollbars:** Custom-styled to be thin, dark gray tracks with slightly lighter gray thumbs to avoid distracting from the content.
- **Breadcrumbs:** Use JetBrains Mono for the path segments, separated by subtle chevron icons.