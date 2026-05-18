import type { ThemePalette } from '../types';

// Claude theme color tokens - warm clay on cream, inspired by Claude Code
export const claudePrimary = '#C96442';
export const claudePrimaryLight = '#D97757';
export const claudePrimaryDark = '#B8552F';

export const claudePalette: ThemePalette = {
  mode: 'light',
  primary: {
    main: claudePrimary,
    light: claudePrimaryLight,
    dark: claudePrimaryDark,
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#8A7860',
    light: '#A89684',
    dark: '#6B5C48',
    contrastText: '#ffffff',
  },
  success: {
    main: '#5B8A5A',
    light: '#7BAA7A',
    dark: '#456A44',
  },
  error: {
    main: '#BF4434',
    light: '#D26659',
    dark: '#9A3527',
  },
  warning: {
    main: '#C2873B',
    light: '#DDA45A',
    dark: '#9A6A2A',
  },
  info: {
    main: '#6A8FA8',
  },
  background: {
    default: '#FAF9F5',
    paper: '#FFFFFF',
  },
  text: {
    primary: '#1F1E1D',
    secondary: '#6B6863',
    disabled: '#A8A39B',
  },
  divider: '#E8E4DA',
  action: {
    hover: '#F2EEE3',
    selected: '#EAE0CF',
    disabled: '#F2EEE3',
    focus: '#EAE0CF',
  },
  dashboard: {
    token: {
      input: { main: claudePrimary, gradient: 'rgba(201, 100, 66, 0.8)' },
      output: { main: '#5B8A5A', gradient: 'rgba(91, 138, 90, 0.8)' },
      cache: { main: '#C9BFAE', gradient: 'rgba(201, 191, 174, 0.7)' },
    },
    chart: {
      grid: '#F2EEE3',
      axis: '#E8E4DA',
      tooltipBg: '#FFFFFF',
      tooltipBorder: '#E8E4DA',
    },
    statCard: {
      boxShadow: '0 2px 4px rgba(31, 30, 29, 0.08)',
      emptyIconBg: 'rgba(201, 100, 66, 0.1)',
    },
  },
  isSunlit: false,
};
