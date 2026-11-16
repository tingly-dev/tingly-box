import type { ThemePalette } from '../types';
import type { ThemeTokens } from '../tokens';

export const lightPalette: ThemePalette = {
  mode: 'light',
  primary: {
    main: '#2563eb',
    light: '#3b82f6',
    dark: '#1d4ed8',
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#64748b',
    light: '#cbd5e1',
    dark: '#475569',
    contrastText: '#ffffff',
  },
  success: {
    main: '#059669',
    light: '#10b981',
    dark: '#047857',
  },
  error: {
    main: '#dc2626',
    light: '#ef4444',
    dark: '#b91c1c',
  },
  warning: {
    main: '#d97706',
    light: '#f59e0b',
    dark: '#b45309',
  },
  info: {
    main: '#0891b2',
  },
  background: {
    default: '#f8fafc',
    paper: '#ffffff',
  },
  text: {
    primary: '#1e293b',
    secondary: '#64748b',
    disabled: '#94a3b8',
  },
  divider: '#e2e8f0',
  action: {
    hover: '#f1f5f9',
    selected: '#e0e7ff',
    // `disabled` is the *foreground* MUI uses for disabled Button/IconButton/
    // Chip text — reuse `text.disabled` above so labels stay legible, rather
    // than the near-white tint this used to hold. That tint moved to
    // `disabledBackground`, the token MUI actually reads for a disabled
    // contained Button's fill. See `.design/theme.md`.
    disabled: '#94a3b8',
    disabledBackground: '#f1f5f9',
    focus: '#e0e7ff',
  },
  dashboard: {
    token: {
      input: { main: '#3B82F6', gradient: 'rgba(59, 130, 246, 0.8)' },
      output: { main: '#10B981', gradient: 'rgba(16, 185, 129, 0.8)' },
      cache: { main: '#cbd5e1', gradient: 'rgba(203, 213, 225, 0.7)' },
    },
    chart: {
      grid: '#f1f5f9',
      axis: '#e2e8f0',
      tooltipBg: '#ffffff',
      tooltipBorder: '#e2e8f0',
    },
    statCard: {
      boxShadow: '0 2px 4px rgba(0, 0, 0, 0.1)',
      emptyIconBg: 'rgba(100, 116, 139, 0.1)',
    },
  },
};

export const lightTokens: ThemeTokens = {
  accent: '#2563eb',
  text: '#1e293b',
  textMuted: '#64748b',
  textHelper: '#6b7280',
  placeholder: '#94a3b8',
  error: '#dc2626',
  surface: {
    card: '#ffffff',
    appBar: '#ffffff',
    menu: '#ffffff',
    popover: '#ffffff',
    dialog: '#ffffff',
    tooltip: '#ffffff',
  },
  border: {
    card: '#e2e8f0',
    chrome: '#e2e8f0',
    menu: '#e2e8f0',
    tooltip: '#e2e8f0',
    divider: '#e2e8f0',
  },
  shadow: {
    menu: '0 10px 24px rgba(15, 23, 42, 0.08)',
    tooltip: '0 4px 12px rgba(15, 23, 42, 0.08)',
  },
  input: {
    bg: 'transparent',
    border: '#d1d5db',
    hoverBorder: '#9ca3af',
    disabledBorder: '#e5e7eb',
    filledBg: 'rgba(0, 0, 0, 0.04)',
    filledHoverBg: 'rgba(0, 0, 0, 0.06)',
    filledFocusBg: 'rgba(0, 0, 0, 0.06)',
  },
  button: {
    outlinedBorder: '#d1d5db',
    outlinedText: '#374151',
    outlinedHoverBorder: '#9ca3af',
    outlinedHoverBg: '#f9fafb',
  },
  menuItem: {
    hover: '#f1f5f9',
    selected: '#e0e7ff',
    selectedHover: '#c7d2fe',
  },
};
