import type { ThemePalette } from '../types';

// DS theme color tokens — deepseek.com's actual brand palette
// (`--ds-color-brand`/`-light-reverse`/`-deep`/`-medium-reverse` in its shipped
// CSS), a single blue hue rather than an invented primary+secondary pair — the
// real site has no purple/secondary accent, so `secondary` below reuses the
// same hue family instead of making one up.
export const dsPrimary = '#4D6BFE';
export const dsPrimaryLight = '#6799FE';
export const dsPrimaryDark = '#3A65C2';

// Matches deepseek.com's actual hero recipe (inspected from its shipped CSS/JS):
// a near-white page base (`--ds-color-bg-page: #f9f8f8`) with a sky-blue wash
// (`linear-gradient(180deg, #9cc1e7 0%, rgba(250,250,250,0) 100%)`) faded in from
// the top, plus an animated canvas "flow field" in `#8AA3D6`/`#9cc1e7`/white for
// the misty texture. We approximate the flow field's organic variation with
// several overlapping soft blobs (static — no canvas/animation, see MuiCssBaseline
// in components/ds.ts) instead of reproducing the single flat diagonal band this
// theme originally shipped with.
export const dsBackgroundGradient = {
  base: '#F9F8F8',
  wash: '#9CC1E7',
  accent: '#8AA3D6',
};

export const dsPalette: ThemePalette = {
  mode: 'light',
  primary: {
    main: dsPrimary,
    light: dsPrimaryLight,
    dark: dsPrimaryDark,
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#73A3D2', // --ds-color-brand-light-reverse
    light: '#9CC1E7', // same wash tone as the hero background
    dark: '#4176E6', // --ds-color-brand-medium-reverse
    contrastText: '#ffffff',
  },
  success: {
    main: '#22c55e',
    light: '#4ade80',
    dark: '#16a34a',
  },
  error: {
    main: '#ef4444',
    light: '#f87171',
    dark: '#dc2626',
  },
  warning: {
    main: '#f59e0b',
    light: '#fbbf24',
    dark: '#d97706',
  },
  info: {
    main: dsPrimaryLight,
  },
  background: {
    default: 'transparent',
    paper: 'rgba(255, 255, 255, 0.88)',
    // @ts-ignore - custom gradient field, painted onto <body> by MuiCssBaseline
    gradient: dsBackgroundGradient,
  },
  text: {
    primary: '#1e232c', // --ds-color-text-primary
    secondary: '#565f70',
    disabled: '#9aa3ac',
  },
  divider: 'rgba(77, 107, 254, 0.12)',
  action: {
    hover: 'rgba(77, 107, 254, 0.08)',
    selected: 'rgba(77, 107, 254, 0.15)',
    disabled: 'rgba(77, 107, 254, 0.04)',
    focus: 'rgba(77, 107, 254, 0.12)',
  },
  dashboard: {
    token: {
      input: { main: dsPrimary, gradient: 'rgba(77, 107, 254, 0.75)' },
      output: { main: '#4176E6', gradient: 'rgba(65, 118, 230, 0.75)' },
      cache: { main: '#94a3b8', gradient: 'rgba(148, 163, 184, 0.65)' },
    },
    chart: {
      grid: 'rgba(77, 107, 254, 0.08)',
      axis: 'rgba(77, 107, 254, 0.15)',
      tooltipBg: 'rgba(255, 255, 255, 0.96)',
      tooltipBorder: 'rgba(77, 107, 254, 0.2)',
    },
    statCard: {
      boxShadow: '0 2px 12px rgba(77, 107, 254, 0.12), 0 1px 4px rgba(0, 0, 0, 0.04)',
      emptyIconBg: 'rgba(77, 107, 254, 0.1)',
    },
  },
  isSunlit: false,
};
