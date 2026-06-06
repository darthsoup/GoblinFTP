---
name: Goblin Tech-Light
colors:
  surface: '#f7f9fb'
  surface-dim: '#d8dadc'
  surface-bright: '#f7f9fb'
  surface-container-lowest: '#ffffff'
  surface-container-low: '#f2f4f6'
  surface-container: '#eceef0'
  surface-container-high: '#e6e8ea'
  surface-container-highest: '#e0e3e5'
  on-surface: '#191c1e'
  on-surface-variant: '#3b4b37'
  inverse-surface: '#2d3133'
  inverse-on-surface: '#eff1f3'
  outline: '#6b7c65'
  outline-variant: '#b9ccb2'
  surface-tint: '#006e16'
  primary: '#006e16'
  on-primary: '#ffffff'
  primary-container: '#00ff41'
  on-primary-container: '#007117'
  inverse-primary: '#00e639'
  secondary: '#565e74'
  on-secondary: '#ffffff'
  secondary-container: '#dae2fd'
  on-secondary-container: '#5c647a'
  tertiary: '#775839'
  on-tertiary: '#ffffff'
  tertiary-container: '#ffd5ae'
  on-tertiary-container: '#7a5b3c'
  error: '#ba1a1a'
  on-error: '#ffffff'
  error-container: '#ffdad6'
  on-error-container: '#93000a'
  primary-fixed: '#72ff70'
  primary-fixed-dim: '#00e639'
  on-primary-fixed: '#002203'
  on-primary-fixed-variant: '#00530e'
  secondary-fixed: '#dae2fd'
  secondary-fixed-dim: '#bec6e0'
  on-secondary-fixed: '#131b2e'
  on-secondary-fixed-variant: '#3f465c'
  tertiary-fixed: '#ffdcbd'
  tertiary-fixed-dim: '#e7bf99'
  on-tertiary-fixed: '#2c1701'
  on-tertiary-fixed-variant: '#5d4124'
  background: '#f7f9fb'
  on-background: '#191c1e'
  surface-variant: '#e0e3e5'
typography:
  display-lg:
    fontFamily: Geist
    fontSize: 48px
    fontWeight: '700'
    lineHeight: 56px
    letterSpacing: -0.02em
  headline-lg:
    fontFamily: Geist
    fontSize: 32px
    fontWeight: '600'
    lineHeight: 40px
    letterSpacing: -0.01em
  headline-lg-mobile:
    fontFamily: Geist
    fontSize: 24px
    fontWeight: '600'
    lineHeight: 32px
  body-md:
    fontFamily: Geist
    fontSize: 16px
    fontWeight: '400'
    lineHeight: 24px
  body-sm:
    fontFamily: Geist
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  code-md:
    fontFamily: JetBrains Mono
    fontSize: 14px
    fontWeight: '500'
    lineHeight: 20px
  label-caps:
    fontFamily: JetBrains Mono
    fontSize: 12px
    fontWeight: '700'
    lineHeight: 16px
rounded:
  sm: 0.125rem
  DEFAULT: 0.25rem
  md: 0.375rem
  lg: 0.5rem
  xl: 0.75rem
  full: 9999px
spacing:
  unit: 4px
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 40px
  gutter: 24px
  margin-mobile: 16px
  margin-desktop: 48px
---

## Brand & Style
The design system transitions the high-energy "Goblin" aesthetic into a professional, high-clarity light environment. It targets developers and technical professionals who require the precision of a dark mode interface but prefer the readability and reduced eye strain of a light-mode workspace. 

The style is **Corporate Modern** with a **Minimalist** lean. It prioritizes white space and sharp typography to balance the aggressive neon accent color. The emotional response is one of surgical precision, modern efficiency, and technological rigor. The interface feels "alive" through the use of a single, high-vibrancy green against a sterile, clinical backdrop.

## Colors
The palette is anchored by **Goblin Green (#00FF41)**, used exclusively for primary actions, success states, and critical data highlights. To maintain professionalism in a light context, this neon green is balanced against a sophisticated "Slate" secondary color used for primary text and iconography.

The background architecture utilizes a layered white and light-gray system. The base surface is pure white, while structural containers and sidebars use a subtle off-white to create functional zoning without heavy shadows. Borders are kept thin and low-contrast to ensure the "Goblin" accent remains the dominant focal point.

## Typography
This design system utilizes a dual-font strategy to reinforce the technical narrative. **Geist** serves as the primary typeface for all UI elements and body copy, providing a clean, geometric, and developer-friendly sans-serif look. 

For technical data, labels, and status indicators, **JetBrains Mono** is employed. This monospaced font provides the necessary "tech" feel and ensures that numerical data remains legible and aligned. Headlines use tight letter-spacing and heavy weights to command attention, while body text maintains generous line-heights for long-form readability.

## Layout & Spacing
The layout follows a **Fixed Grid** philosophy on desktop to maintain a controlled, dashboard-like density. A 12-column system is used for page layouts, while a 4px base unit (the "Tech-Step") governs all internal component spacing.

On mobile, the grid collapses to a single column with 16px side margins. Tablet transitions use an 8-column fluid approach. Vertical rhythm is strictly enforced using the 4px increments to ensure that technical components, such as data tables and code blocks, feel mathematically aligned.

## Elevation & Depth
Depth in this design system is achieved through **Tonal Layers** rather than traditional shadows. Surfaces are stacked using color shifts:
- **Level 0 (Background):** Slate-50 (#F8FAFC)
- **Level 1 (Cards/Content):** Pure White (#FFFFFF)
- **Level 2 (Overlays/Popovers):** Pure White with a 1px border of Slate-200.

Shadows are used sparingly and only for floating elements (modals, dropdowns). When applied, they are "Ambient Shadows"—ultra-diffused, 10% opacity, and slightly tinted with the secondary color to prevent a "muddy" look on the light background.

## Shapes
The shape language is **Soft (0.25rem)**. This slight rounding takes the "edge" off the clinical white background while maintaining a professional, structured appearance. 

Buttons and input fields utilize the standard 4px radius. Status tags and "chips" may use the `rounded-xl` (12px) setting to provide a visual distinction from actionable buttons. Large containers like cards and panels should strictly adhere to the 4px (Soft) radius to maintain the architectural integrity of the grid.

## Components
- **Buttons:** Primary buttons use a solid Goblin Green (#00FF41) background with the Secondary (#0F172A) text for maximum contrast. Secondary buttons use a transparent background with a 1px border of Slate-200.
- **Input Fields:** Use a 1px Slate-200 border. On focus, the border transitions to Goblin Green with a subtle 2px outer glow in the same color (20% opacity).
- **Cards:** White backgrounds with a 1px border. No shadows unless they are interactive or "lifted" on hover.
- **Chips/Badges:** For status, use a 10% opacity Goblin Green fill with 100% opacity Green text. This ensures the neon color is legible as text on a light background.
- **Data Tables:** Use horizontal rules only (Slate-100). Header rows should have a subtle Slate-50 background tint.
- **Code Blocks:** Use a slightly darker neutral background (#F1F5F9) to provide a "recessed" look, utilizing JetBrains Mono for the content.