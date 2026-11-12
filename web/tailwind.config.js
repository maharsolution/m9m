/**
 * m9m design foundation — Tailwind token layer.
 *
 * Source of truth: DESIGN.md (repo root). Values are adopted verbatim from
 * n8n's design-system primitives (`n8n/packages/frontend/@n8n/design-system/src/css/_primitives.scss`)
 * so the two products stay visually identical.
 *
 * Contract published to component authors:
 *   colors   brand (Signal Pink), neutral (Graphite), accent (Ember Orange),
 *            primary (alias of brand), green/yellow/red/blue (n8n ramps),
 *            purple + mint (n8n ramps), slate (legacy only)
 *   type     font-sans / font-mono, text-4xs…text-2xl, font-regular…font-semibold
 *   radius   rounded-xs…rounded-4xl, rounded-full
 *   shadow   shadow-2xs / shadow-xs / shadow-sm / shadow-md / shadow-xl / shadow-outline
 *   motion   duration-snappy, ease-out
 */

/**
 * Makes every ramp step opacity-modifier capable (`bg-brand-500/20`).
 * Tailwind replaces the placeholder with the requested alpha, so the
 * published oklch values are preserved for the un-modified case.
 */
const ramp = (scale) =>
  Object.fromEntries(
    Object.entries(scale).map(([step, value]) => [step, value.replace(/\)$/, ' / <alpha-value>)')]),
  )

/** Signal Pink — the m9m brand ramp (n8n pink). */
const brand = ramp({
  50: 'oklch(97.53% 0.0128 5.65)',
  100: 'oklch(95.06% 0.0256 5.65)',
  150: 'oklch(92.56% 0.0398 5.49)',
  200: 'oklch(90.05% 0.0541 5.33)',
  250: 'oklch(88.05% 0.0666 5.18)',
  300: 'oklch(86.04% 0.0791 5.02)',
  400: 'oklch(82.05% 0.1071 4.98)',
  500: 'oklch(78.07% 0.136 5.02)',
  600: 'oklch(68.08% 0.148 5.02)',
  700: 'oklch(59.09% 0.1611 5.09)',
  800: 'oklch(40.08% 0.1209 4.81)',
  900: 'oklch(24.87% 0.1 4.8)',
  950: 'oklch(18% 0.075 4.8)',
})

/** Graphite Neutral — the surface / text ramp. */
const neutral = ramp({
  white: 'oklch(100% 0 89.88)',
  50: 'oklch(98.51% 0 89.88)',
  100: 'oklch(97.02% 0 89.88)',
  150: 'oklch(95.51% 0 89.88)',
  200: 'oklch(92.19% 0 89.88)',
  250: 'oklch(90.67% 0 89.88)',
  300: 'oklch(88.53% 0 89.88)',
  400: 'oklch(79.21% 0 89.88)',
  500: 'oklch(68.3% 0 89.88)',
  600: 'oklch(55.55% 0 89.88)',
  700: 'oklch(38.67% 0 89.88)',
  800: 'oklch(31.71% 0 89.88)',
  900: 'oklch(26.86% 0 89.88)',
  950: 'oklch(20.46% 0 89.88)',
})

/** Ember Orange — the inline-emphasis accent. */
const accent = ramp({
  50: 'oklch(97.96% 0.0158 73.68)',
  100: 'oklch(95.4% 0.0382 76.2)',
  150: 'oklch(95.4% 0.0382 76.2)',
  200: 'oklch(89.99% 0.0763 70.44)',
  250: 'oklch(89.99% 0.0763 70.44)',
  300: 'oklch(83.37% 0.1256 67.19)',
  400: 'oklch(74.58% 0.1802 56.73)',
  500: 'oklch(69.96% 0.202 44.44)',
  600: 'oklch(64.71% 0.2173 36.85)',
  700: 'oklch(55.42% 0.1922 35.48)',
  800: 'oklch(46.96% 0.1566 37.15)',
  900: 'oklch(40.73% 0.1228 38.02)',
  950: 'oklch(26.62% 0.0789 35.91)',
})

const green = ramp({
  50: 'oklch(98.19% 0.0181 155.83)',
  100: 'oklch(96.24% 0.0434 156.74)',
  150: 'oklch(94.08% 0.0693 155.66)',
  200: 'oklch(92.58% 0.0845 155.86)',
  250: 'oklch(88.88% 0.1311 154.31)',
  300: 'oklch(87.04% 0.149 154.62)',
  400: 'oklch(79.14% 0.2091 151.66)',
  500: 'oklch(72.89% 0.2119 147.82)',
  600: 'oklch(63.2% 0.186 147.37)',
  700: 'oklch(52.96% 0.1495 148.99)',
  800: 'oklch(44.68% 0.1187 151.4)',
  900: 'oklch(39.35% 0.0957 152.28)',
  950: 'oklch(26.54% 0.0651 152.77)',
})

const yellow = ramp({
  50: 'oklch(98.73% 0.0262 102.21)',
  100: 'oklch(97.27% 0.0706 103.29)',
  150: 'oklch(95.75% 0.1125 103.41)',
  200: 'oklch(94.53% 0.1293 101.4)',
  250: 'oklch(93.07% 0.1604 101.32)',
  300: 'oklch(90.32% 0.1817 98.27)',
  400: 'oklch(85.35% 0.1744 88.77)',
  500: 'oklch(79.77% 0.1641 82.71)',
  600: 'oklch(68.22% 0.1465 71.47)',
  700: 'oklch(55.48% 0.1271 62.52)',
  800: 'oklch(47.76% 0.1115 60.43)',
  900: 'oklch(42.15% 0.0952 57.94)',
  950: 'oklch(28.71% 0.0662 54.59)',
})

const red = ramp({
  50: 'oklch(97.05% 0.0129 17.38)',
  100: 'oklch(93.65% 0.032 17.74)',
  150: 'oklch(91.08% 0.046 18.03)',
  200: 'oklch(88.34% 0.0616 18.39)',
  250: 'oklch(84.85% 0.0827 18.93)',
  300: 'oklch(80.53% 0.1109 19.78)',
  400: 'oklch(70.22% 0.1892 22.23)',
  500: 'oklch(63.78% 0.2373 25.44)',
  600: 'oklch(58.3% 0.2387 28.48)',
  700: 'oklch(50.95% 0.2086 28.51)',
  800: 'oklch(44.46% 0.1774 26.79)',
  900: 'oklch(39.67% 0.1408 25.71)',
  950: 'oklch(25.75% 0.0918 25.84)',
})

const blue = ramp({
  50: 'oklch(97.05% 0.0142 254.6)',
  100: 'oklch(93.19% 0.0316 255.59)',
  150: 'oklch(90.6% 0.0446 255.63)',
  200: 'oklch(88.2% 0.0588 253.97)',
  250: 'oklch(83.4% 0.0842 254.17)',
  300: 'oklch(80.71% 0.1007 250.45)',
  400: 'oklch(70.4% 0.1583 253.63)',
  500: 'oklch(61.87% 0.2067 259.23)',
  600: 'oklch(54.65% 0.2455 262.87)',
  700: 'oklch(48.78% 0.2432 264.4)',
  800: 'oklch(42.42% 0.1982 265.5)',
  900: 'oklch(37.96% 0.1453 265.47)',
  950: 'oklch(28.1% 0.0924 268.13)',
})

/** Secondary / violet family — n8n ramp, used for inline emphasis. */
const purple = ramp({
  50: 'oklch(96.91% 0.0161 293.76)',
  100: 'oklch(94.33% 0.0284 294.59)',
  150: 'oklch(94.33% 0.0284 294.59)',
  200: 'oklch(89.46% 0.0562 293.06)',
  250: 'oklch(89.46% 0.0562 293.06)',
  300: 'oklch(81.01% 0.1055 293.7)',
  400: 'oklch(69.89% 0.176 294.12)',
  500: 'oklch(60.06% 0.2425 293.89)',
  600: 'oklch(54.05% 0.2811 292.85)',
  700: 'oklch(49.07% 0.2699 292.56)',
  800: 'oklch(43.16% 0.232 292.71)',
  900: 'oklch(38.01% 0.1888 293.64)',
  950: 'oklch(28.4% 0.1414 291.29)',
})

/** Mint — n8n node-icon palette. */
const mint = ramp({
  50: 'oklch(98.36% 0.0142 180.72)',
  100: 'oklch(95.2% 0.0507 181.07)',
  150: 'oklch(91.84% 0.0887 179.84)',
  200: 'oklch(91.08% 0.0962 180.11)',
  250: 'oklch(89.1% 0.1141 179.41)',
  300: 'oklch(85.39% 0.1375 181.57)',
  400: 'oklch(78.3% 0.1408 181.38)',
  500: 'oklch(71.05% 0.1276 181.6)',
  600: 'oklch(60.49% 0.107 184.17)',
  700: 'oklch(51.52% 0.0905 185.73)',
  800: 'oklch(43.76% 0.0759 188.55)',
  900: 'oklch(38.71% 0.0634 187.21)',
  950: 'oklch(27.66% 0.046 192.53)',
})

/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{vue,js,ts,jsx,tsx}',
  ],
  /*
   * The semantic utility classes published by `main.css` live in
   * `@layer utilities`, which Tailwind tree-shakes when unused. They are part
   * of the public contract, so they are safelisted and always shipped.
   * The shared component classes from `@layer components` are safelisted for
   * the same reason: pages rely on them without importing anything.
   */
  safelist: [
    // Semantic utilities.
    'bg-surface',
    'bg-subtle',
    'bg-hover',
    'bg-active',
    'bg-brand',
    'text-primary',
    'text-secondary',
    'text-tertiary',
    'text-disabled',
    'border-default',
    'border-subtle',
    'border-strong',
    'animate-in',
    'text-balance',
    'no-scrollbar',
    // Shared component classes.
    'btn',
    'btn-primary',
    'btn-secondary',
    'btn-ghost',
    'btn-danger',
    'btn-icon',
    'input',
    'input-error',
    'card',
    'card-hover',
    'panel',
    'badge',
    'badge-success',
    'badge-warning',
    'badge-error',
    'badge-info',
    'workflow-node',
    'workflow-node-trigger',
    'workflow-node-action',
    'workflow-node-transform',
    'connection-handle',
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        /* Brand — Signal Pink. */
        brand,
        /* Legacy alias: `primary-*` now resolves to the brand ramp. */
        primary: brand,
        /* Graphite Neutral — the surface / text ramp. */
        neutral,
        /* Ember Orange — inline emphasis accent. */
        accent,
        /* Status ramps (n8n parity). */
        green,
        yellow,
        red,
        blue,
        /* Secondary + node-icon families (n8n parity). */
        purple,
        mint,
        /* Legacy category colors, remapped onto the new ramps. */
        trigger: {
          light: green[500],
          dark: green[300],
        },
        action: {
          light: blue[500],
          dark: blue[300],
        },
        transform: {
          light: yellow[500],
          dark: yellow[300],
        },
        canvas: {
          light: 'hsl(0 0% 96% / <alpha-value>)',
          dark: neutral[950],
          grid: {
            light: neutral[200],
            dark: neutral[800],
          },
        },
        surface: {
          light: neutral.white,
          dark: neutral[800],
        },
      },
      fontFamily: {
        sans: ['Geist', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['Geist Mono', 'ui-monospace', 'Menlo', 'Consolas', 'DejaVu Sans Mono', 'monospace'],
      },
      fontSize: {
        '4xs': ['8px', { lineHeight: '1' }],
        '3xs': ['10px', { lineHeight: '1.25' }],
        '2xs': ['12px', { lineHeight: '1.25' }],
        xs: ['13px', { lineHeight: '1.5' }],
        sm: ['14px', { lineHeight: '1.5' }],
        base: ['16px', { lineHeight: '1.3' }],
        lg: ['18px', { lineHeight: '1.35' }],
        xl: ['20px', { lineHeight: '1.35' }],
        '2xl': ['28px', { lineHeight: '1.5' }],
      },
      fontWeight: {
        regular: '450',
        medium: '500',
        semibold: '600',
        bold: '600',
      },
      letterSpacing: {
        tightest: '-0.025em',
        tighter: '-0.015em',
        tight: '-0.007em',
        normal: '0',
        wide: '0.025em',
        wider: '0.05em',
        widest: '0.1em',
      },
      borderRadius: {
        xs: '2px',
        sm: '4px',
        md: '6px',
        lg: '8px',
        xl: '12px',
        '2xl': '16px',
        '3xl': '18px',
        '4xl': '20px',
        full: '9999px',
      },
      boxShadow: {
        '2xs': '0 1px oklch(0% 0 0 / 0.1)',
        xs: '0 1px 2px -1px oklch(0% 0 0 / 0.1)',
        sm: '0 4px 6px -1px oklch(0% 0 0 / 0.1), 0 2px 4px -2px oklch(0% 0 0 / 0.1)',
        md: '0 10px 15px -3px oklch(0% 0 0 / 0.1), 0 4px 6px -4px oklch(0% 0 0 / 0.1)',
        xl: '0 20px 25px -5px oklch(0% 0 0 / 0.1), 0 8px 10px -6px oklch(0% 0 0 / 0.1)',
        outline: '0 0 0 1px oklch(0% 0 0 / 0.1)',
      },
      transitionDuration: {
        snappy: '200ms',
        base: '400ms',
        slow: '1000ms',
        slowest: '1500ms',
      },
      transitionTimingFunction: {
        out: 'cubic-bezier(0.215, 0.61, 0.355, 1)',
        'ease-out': 'cubic-bezier(0.215, 0.61, 0.355, 1)',
        'ease-in': 'cubic-bezier(0.55, 0.055, 0.675, 0.19)',
        'ease-in-out': 'cubic-bezier(0.645, 0.045, 0.355, 1)',
        spring: 'cubic-bezier(0.34, 1.56, 0.64, 1)',
        'ease-out-quart': 'cubic-bezier(0.165, 0.84, 0.44, 1)',
        'ease-out-expo': 'cubic-bezier(0.19, 1, 0.22, 1)',
      },
      animation: {
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'slide-in': 'slideIn 0.2s cubic-bezier(0.215, 0.61, 0.355, 1)',
        'fade-in': 'fadeIn 0.15s cubic-bezier(0.215, 0.61, 0.355, 1)',
      },
      keyframes: {
        slideIn: {
          '0%': { transform: 'translateX(-10px)', opacity: '0' },
          '100%': { transform: 'translateX(0)', opacity: '1' },
        },
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
      },
    },
  },
  plugins: [],
}
