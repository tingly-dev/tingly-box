import { createTheme, type ThemeOptions } from '@mui/material/styles';

// Sunlit theme palette - sky blue tones, clear and bright like a sunny day
const SUNLIT_PALETTE = {
  primary: {
    main: '#0ea5e9', // Sky blue - clear and bright
    light: '#38bdf8',
    dark: '#0284c7',
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#6366f1', // Soft indigo - like distant mountains
    light: '#818cf8',
    dark: '#4f46e5',
    contrastText: '#ffffff',
  },
  background: {
    default: 'transparent',
    paper: 'rgba(255, 255, 255, 0.75)',
    paperSolid: 'rgba(255, 255, 255, 0.92)',
    gradient: {
      start: '#e0f2fe', // sky-50 - lightest sky blue
      middle: '#bae6fd', // sky-200 - medium sky blue
      end: '#7dd3fc', // sky-300 - deeper sky blue
    },
  },
  // Dashboard token colors for sunlit theme - sky and cloud inspired
  dashboard: {
    token: {
      input: {
        main: '#0ea5e9', // Sky blue
        gradient: 'rgba(14, 165, 233, 0.75)',
      },
      output: {
        main: '#22d3ee', // Cyan - bright sky
        gradient: 'rgba(34, 211, 238, 0.75)',
      },
      cache: {
        main: '#94a3b8', // Cloud gray - soft and neutral
        gradient: 'rgba(148, 163, 184, 0.65)',
      },
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
};

const DARK_DASHBOARD_COLORS = {
  token: {
    input: {
      main: '#60A5FA',
      gradient: 'rgba(96, 165, 250, 0.8)',
    },
    output: {
      main: '#34D399',
      gradient: 'rgba(52, 211, 153, 0.8)',
    },
    cache: {
      main: '#94a3b8',
      gradient: 'rgba(148, 163, 184, 0.7)',
    },
  },
  chart: {
    grid: 'rgba(255, 255, 255, 0.08)',
    axis: 'rgba(255, 255, 255, 0.2)',
    tooltipBg: '#1e293b',
    tooltipBorder: '#334155',
  },
  statCard: {
    boxShadow: '0 2px 4px rgba(0, 0, 0, 0.2)',
    emptyIconBg: 'rgba(148, 163, 184, 0.1)',
  },
};

const LIGHT_DASHBOARD_COLORS = {
  token: {
    input: {
      main: '#3B82F6',
      gradient: 'rgba(59, 130, 246, 0.8)',
    },
    output: {
      main: '#10B981',
      gradient: 'rgba(16, 185, 129, 0.8)',
    },
    cache: {
      main: '#cbd5e1',
      gradient: 'rgba(203, 213, 225, 0.7)',
    },
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
};

const getThemeOptions = (mode: 'light' | 'dark' | 'sunlit'): ThemeOptions => {
  const isDark = mode === 'dark';
  const isSunlit = mode === 'sunlit';

  // Use sunlit palette for sunlit theme, otherwise use standard palette
  const primaryColor = isSunlit ? SUNLIT_PALETTE.primary : {
    main: '#2563eb',
    light: '#3b82f6',
    dark: '#1d4ed8',
    contrastText: '#ffffff',
  };

  const secondaryColor = isSunlit ? SUNLIT_PALETTE.secondary : {
    main: isDark ? '#94a3b8' : '#64748b',
    light: '#cbd5e1',
    dark: '#475569',
    contrastText: '#ffffff',
  };

  const backgroundColor = isSunlit ? SUNLIT_PALETTE.background : {
    default: isDark ? '#0f172a' : '#f8fafc',
    paper: isDark ? '#1e293b' : '#ffffff',
  };

  // Text colors - clear and fresh for sky blue theme
  const textPrimary = isSunlit ? '#0f172a' : (isDark ? '#f8fafc' : '#1e293b');
  const textSecondary = isSunlit ? '#475569' : (isDark ? '#cbd5e1' : '#64748b');
  const textDisabled = isSunlit ? '#94a3b8' : (isDark ? '#94a3b8' : '#94a3b8');

  const dividerColor = isSunlit ? 'rgba(14, 165, 233, 0.12)' : (isDark ? '#334155' : '#e2e8f0');

  // Dashboard-specific colors
  const dashboardColors = isSunlit
    ? SUNLIT_PALETTE.dashboard
    : (isDark ? DARK_DASHBOARD_COLORS : LIGHT_DASHBOARD_COLORS);

  // Common colors for sunlit theme
  const sunlitPrimary = '#0ea5e9';
  const sunlitPrimaryLight = '#38bdf8';
  const sunlitPrimaryDark = '#0284c7';

  return {
    palette: {
      mode: isSunlit ? 'light' : mode,
      primary: primaryColor,
      secondary: secondaryColor,
      // Add identifier for sunlit theme
      isSunlit: isSunlit,
      success: {
        main: isSunlit ? '#22c55e' : '#059669',
        light: isSunlit ? '#4ade80' : '#10b981',
        dark: isSunlit ? '#16a34a' : '#047857',
      },
      error: {
        main: isSunlit ? '#ef4444' : '#dc2626',
        light: isSunlit ? '#f87171' : '#ef4444',
        dark: isSunlit ? '#dc2626' : '#b91c1c',
      },
      warning: {
        main: isSunlit ? '#f59e0b' : '#d97706',
        light: isSunlit ? '#fbbf24' : '#f59e0b',
        dark: isSunlit ? '#d97706' : '#b45309',
      },
      info: {
        main: isSunlit ? '#06b6d4' : '#0891b2',
      },
      background: backgroundColor,
      text: {
        primary: textPrimary,
        secondary: textSecondary,
        disabled: textDisabled,
      },
      divider: dividerColor,
      action: {
        hover: isSunlit ? 'rgba(14, 165, 233, 0.08)' : (isDark ? '#1e293b' : '#f1f5f9'),
        selected: isSunlit ? 'rgba(14, 165, 233, 0.15)' : (isDark ? '#1e3a8a' : '#e0e7ff'),
        disabled: isSunlit ? 'rgba(14, 165, 233, 0.04)' : (isDark ? '#1e293b' : '#f1f5f9'),
      },
      // Dashboard colors palette
      dashboard: {
        token: dashboardColors.token,
        chart: dashboardColors.chart,
        statCard: dashboardColors.statCard,
      },
    },
    typography: {
      fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Helvetica", "Arial", sans-serif',
      h1: {
        fontSize: '2rem',
        fontWeight: 600,
        color: textPrimary,
      },
      h2: {
        fontSize: '1.5rem',
        fontWeight: 600,
        color: textPrimary,
      },
      h3: {
        fontSize: '1.25rem',
        fontWeight: 600,
        color: textPrimary,
      },
      h4: {
        fontSize: '1.125rem',
        fontWeight: 600,
        color: textPrimary,
      },
      h5: {
        fontSize: '1rem',
        fontWeight: 600,
        color: textPrimary,
      },
      h6: {
        fontSize: '0.875rem',
        fontWeight: 600,
        color: textPrimary,
      },
      body1: {
        fontSize: '0.875rem',
        color: textSecondary,
      },
      body2: {
        fontSize: '0.75rem',
        color: textSecondary,
      },
      caption: {
        fontSize: '0.625rem',
        color: textDisabled,
      },
    },
    shape: {
      borderRadius: 8,
    },
    components: {
      MuiCard: {
        styleOverrides: {
          root: {
            boxShadow: isSunlit
              ? '0 2px 16px rgba(14, 165, 233, 0.12), 0 1px 6px rgba(0, 0, 0, 0.04)'
              : (isDark
                ? '0 1px 3px 0 rgba(0, 0, 0, 0.3), 0 1px 2px 0 rgba(0, 0, 0, 0.2)'
                : '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'),
            borderRadius: 12,
            border: isSunlit
              ? '1px solid rgba(14, 165, 233, 0.15)'
              : (isDark ? '1px solid #334155' : '1px solid #e2e8f0'),
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.82)'
              : (isDark ? '#1e293b' : '#ffffff'),
            backdropFilter: isSunlit ? 'blur(12px)' : 'none',
          },
        },
      },
      MuiListItemButton: {
        styleOverrides: {
          root: {
            '&.nav-item-active': {
              backgroundColor: isSunlit ? sunlitPrimary : '#2563eb',
              color: '#ffffff',
              '&:hover': {
                backgroundColor: isSunlit ? sunlitPrimaryDark : '#1d4ed8',
              },
              '& .MuiListItemIcon-root': {
                color: '#ffffff',
              },
              '& .MuiListItemText-primary': {
                color: '#ffffff',
                fontWeight: 600,
              },
            },
          },
        },
      },
      MuiButton: {
        styleOverrides: {
          root: {
            textTransform: 'none',
            fontWeight: 500,
            borderRadius: 6,
            boxShadow: 'none',
            '&:hover': {
              boxShadow: isSunlit
                ? '0 2px 8px rgba(14, 165, 233, 0.2)'
                : (isDark
                  ? '0 1px 2px 0 rgba(0, 0, 0, 0.3)'
                  : '0 1px 2px 0 rgba(0, 0, 0, 0.05)'),
            },
          },
          contained: {
            background: isSunlit
              ? `linear-gradient(135deg, ${sunlitPrimary} 0%, ${sunlitPrimaryDark} 100%)`
              : 'linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%)',
            '&:hover': {
              background: isSunlit
                ? `linear-gradient(135deg, ${sunlitPrimaryDark} 0%, #0369a1 100%)`
                : 'linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%)',
            },
          },
          outlined: {
            borderColor: isSunlit ? 'rgba(14, 165, 233, 0.3)' : (isDark ? '#475569' : '#d1d5db'),
            color: isSunlit ? '#0369a1' : (isDark ? '#cbd5e1' : '#374151'),
            '&:hover': {
              borderColor: isSunlit ? 'rgba(14, 165, 233, 0.5)' : (isDark ? '#64748b' : '#9ca3af'),
              backgroundColor: isSunlit ? 'rgba(14, 165, 233, 0.08)' : (isDark ? '#334155' : '#f9fafb'),
            },
          },
        },
      },
      MuiTextField: {
        styleOverrides: {
          root: {
            '& .MuiOutlinedInput-root': {
              borderRadius: 6,
              backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.6)' : 'transparent',
              '& fieldset': {
                borderColor: isSunlit ? 'rgba(14, 165, 233, 0.25)' : (isDark ? '#475569' : '#d1d5db'),
              },
              '&:hover fieldset': {
                borderColor: isSunlit ? 'rgba(14, 165, 233, 0.4)' : (isDark ? '#64748b' : '#9ca3af'),
              },
              '&.Mui-focused fieldset': {
                borderColor: isSunlit ? sunlitPrimary : '#2563eb',
                borderWidth: 1.5,
              },
            },
          },
        },
      },
      MuiSelect: {
        styleOverrides: {
          root: {
            '& .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? 'rgba(14, 165, 233, 0.25)' : (isDark ? '#475569' : '#d1d5db'),
            },
            '&:hover .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? 'rgba(14, 165, 233, 0.4)' : (isDark ? '#64748b' : '#9ca3af'),
            },
            '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? sunlitPrimary : '#2563eb',
              borderWidth: 1.5,
            },
          },
        },
      },
      MuiChip: {
        styleOverrides: {
          root: {
            fontWeight: 500,
            borderRadius: 4,
          },
        },
      },
      MuiAlert: {
        styleOverrides: {
          root: {
            borderRadius: 6,
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.92)' : undefined,
          },
        },
      },
      MuiDrawer: {
        styleOverrides: {
          paper: {
            borderRight: isSunlit ? '1px solid rgba(14, 165, 233, 0.15)' : (isDark ? '1px solid #334155' : '1px solid #e2e8f0'),
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.72)' : undefined,
            // Use lighter blur for better performance
            backdropFilter: isSunlit ? 'blur(8px)' : 'none',
            willChange: 'auto',
          },
        },
      },
      MuiTabs: {
        styleOverrides: {
          indicator: {
            height: 4,
            borderRadius: 2,
            backgroundColor: isSunlit ? sunlitPrimary : '#2563eb',
          },
        },
      },
      MuiPaper: {
        styleOverrides: {
          root: {
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.65)' : undefined,
            // Use lighter blur for better performance
            backdropFilter: isSunlit ? 'blur(8px)' : 'none',
            willChange: 'auto',
          },
        },
      },
    },
  };
};

const createAppTheme = (mode: 'light' | 'dark' | 'sunlit') => {
  return createTheme(getThemeOptions(mode));
};

export default createAppTheme;
