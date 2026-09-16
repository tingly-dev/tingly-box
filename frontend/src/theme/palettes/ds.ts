import type { ThemePalette } from '../types';

// DS theme color tokens — soft indigo-blue over a misty gradient,
// inspired by deepseek.com's hero background.
export const dsPrimary = '#4F6EF7';
export const dsPrimaryLight = '#7B93FA';
export const dsPrimaryDark = '#3B54D4';

export const dsBackgroundGradient = {
  start: '#F4F6FC',
  middle: '#DCE3F5',
  end: '#B9C6EC',
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
    main: '#8B5CF6',
    light: '#A78BFA',
    dark: '#6D28D9',
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
    main: '#6A9BFF',
  },
  background: {
    default: 'transparent',
    paper: 'rgba(255, 255, 255, 0.88)',
    // @ts-ignore - custom gradient field, painted onto <body> by MuiCssBaseline
    gradient: dsBackgroundGradient,
  },
  text: {
    primary: '#1e2338',
    secondary: '#565f7e',
    disabled: '#9aa3c2',
  },
  divider: 'rgba(79, 110, 247, 0.12)',
  action: {
    hover: 'rgba(79, 110, 247, 0.08)',
    selected: 'rgba(79, 110, 247, 0.15)',
    disabled: 'rgba(79, 110, 247, 0.04)',
    focus: 'rgba(79, 110, 247, 0.12)',
  },
  dashboard: {
    token: {
      input: { main: dsPrimary, gradient: 'rgba(79, 110, 247, 0.75)' },
      output: { main: '#8B5CF6', gradient: 'rgba(139, 92, 246, 0.75)' },
      cache: { main: '#94a3b8', gradient: 'rgba(148, 163, 184, 0.65)' },
    },
    chart: {
      grid: 'rgba(79, 110, 247, 0.08)',
      axis: 'rgba(79, 110, 247, 0.15)',
      tooltipBg: 'rgba(255, 255, 255, 0.96)',
      tooltipBorder: 'rgba(79, 110, 247, 0.2)',
    },
    statCard: {
      boxShadow: '0 2px 12px rgba(79, 110, 247, 0.12), 0 1px 4px rgba(0, 0, 0, 0.04)',
      emptyIconBg: 'rgba(79, 110, 247, 0.1)',
    },
  },
  isSunlit: false,
};
