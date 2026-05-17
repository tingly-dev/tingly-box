import type { ThemePalette } from '../types';

// Sunlit theme color tokens - sky blue tones, clear and bright like a sunny day
export const sunlitPrimary = '#0ea5e9';
export const sunlitPrimaryLight = '#38bdf8';
export const sunlitPrimaryDark = '#0284c7';

export const sunlitBackgroundGradient = {
  start: '#e0f2fe',  // sky-50
  middle: '#bae6fd', // sky-200
  end: '#7dd3fc',    // sky-300
};

export const sunlitPalette: ThemePalette = {
  mode: 'light', // Sunlit is a light variant
  primary: {
    main: sunlitPrimary,
    light: sunlitPrimaryLight,
    dark: sunlitPrimaryDark,
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#6366f1', // Soft indigo - like distant mountains
    light: '#818cf8',
    dark: '#4f46e5',
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
    main: '#06b6d4',
  },
  background: {
    default: 'transparent',
    paper: 'rgba(255, 255, 255, 0.75)',
    // @ts-ignore - custom gradient field
    gradient: sunlitBackgroundGradient,
  },
  text: {
    primary: '#0f172a',
    secondary: '#475569',
    disabled: '#94a3b8',
  },
  divider: 'rgba(14, 165, 233, 0.12)',
  action: {
    hover: 'rgba(14, 165, 233, 0.08)',
    selected: 'rgba(14, 165, 233, 0.15)',
    disabled: 'rgba(14, 165, 233, 0.04)',
    focus: 'rgba(14, 165, 233, 0.12)',
  },
  dashboard: {
    token: {
      input: { main: sunlitPrimary, gradient: 'rgba(14, 165, 233, 0.75)' },
      output: { main: '#22d3ee', gradient: 'rgba(34, 211, 238, 0.75)' },
      cache: { main: '#94a3b8', gradient: 'rgba(148, 163, 184, 0.65)' },
    },
    chart: {
      grid: 'rgba(14, 165, 233, 0.08)',
      axis: 'rgba(14, 165, 233, 0.15)',
      tooltipBg: 'rgba(255, 255, 255, 0.96)',
      tooltipBorder: 'rgba(14, 165, 233, 0.2)',
    },
    statCard: {
      boxShadow: '0 2px 12px rgba(14, 165, 233, 0.12), 0 1px 4px rgba(0, 0, 0, 0.04)',
      emptyIconBg: 'rgba(14, 165, 233, 0.1)',
    },
  },
  isSunlit: true,
};
