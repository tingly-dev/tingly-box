import type { ThemePalette } from '../types';

// DS theme color tokens — built from deepseek.com's actual brand blue
// (`--ds-color-brand: #4d6bfe`, `-light-reverse`, `-deep` in its shipped CSS).
// The real site only ever uses that full-saturation blue on small, high-contrast
// surfaces (a button's fill, white text on top) — never as a thin outline against
// a large pastel area. This app's shared hover/selection styling does exactly
// that (`borderColor: 'primary.main'` on card hover), so the raw brand blue read
// as a harsh neon ring against the soft misty background. `dsPrimary` is a step
// softer than the real brand color for that reason; the true saturated brand
// blue survives as `dsPrimaryDark`, for anything that still wants the punch.
//
// The site itself has no second accent hue (it leans on neutrals instead), but
// reusing blue everywhere here read as monochrome/flat once applied across a
// whole admin UI — chart series in particular need to be tellable apart.
// `secondary` is a teal, analogous to the brand blue (same cool family, so it
// stays harmonious) but distinct enough to carry contrast on its own.
export const dsPrimary = '#6799FE';
export const dsPrimaryLight = '#93B4FF';
export const dsPrimaryDark = '#4D6BFE'; // the real, full-saturation deepseek.com brand blue

export const dsSecondary = '#14B8A6';
export const dsSecondaryLight = '#5EEAD4';
export const dsSecondaryDark = '#0F9488';

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
    main: dsSecondary,
    light: dsSecondaryLight,
    dark: dsSecondaryDark,
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
  divider: 'rgba(103, 153, 254, 0.12)',
  action: {
    hover: 'rgba(103, 153, 254, 0.08)',
    selected: 'rgba(103, 153, 254, 0.15)',
    disabled: 'rgba(103, 153, 254, 0.04)',
    focus: 'rgba(103, 153, 254, 0.12)',
  },
  dashboard: {
    token: {
      input: { main: dsPrimary, gradient: 'rgba(103, 153, 254, 0.75)' },
      output: { main: dsSecondary, gradient: 'rgba(20, 184, 166, 0.75)' },
      cache: { main: '#94a3b8', gradient: 'rgba(148, 163, 184, 0.65)' },
    },
    chart: {
      grid: 'rgba(103, 153, 254, 0.08)',
      axis: 'rgba(103, 153, 254, 0.15)',
      tooltipBg: 'rgba(255, 255, 255, 0.96)',
      tooltipBorder: 'rgba(103, 153, 254, 0.2)',
    },
    statCard: {
      boxShadow: '0 2px 12px rgba(103, 153, 254, 0.12), 0 1px 4px rgba(0, 0, 0, 0.04)',
      emptyIconBg: 'rgba(103, 153, 254, 0.1)',
    },
  },
  isSunlit: false,
};
