import { createTheme, type ThemeOptions } from '@mui/material/styles';

// Sunlit theme palette - warm, sunny tones
const SUNLIT_PALETTE = {
  primary: {
    main: '#f59e0b', // Warm amber - softer
    light: '#fbbf24',
    dark: '#d97706',
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#059669', // Fresh green - matches leaves
    light: '#10b981',
    dark: '#047857',
    contrastText: '#ffffff',
  },
  background: {
    default: 'transparent',
    paper: 'rgba(255, 255, 255, 0.7)',
    paperSolid: 'rgba(255, 255, 255, 0.9)',
  },
  // Dashboard token colors for sunlit theme
  dashboard: {
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
        main: '#d4a373', // Warmer cache color for sunlit
        gradient: 'rgba(212, 163, 115, 0.7)',
      },
    },
    chart: {
      grid: 'rgba(0, 0, 0, 0.06)',
      axis: 'rgba(0, 0, 0, 0.1)',
      tooltipBg: 'rgba(255, 255, 255, 0.95)',
      tooltipBorder: 'rgba(0, 0, 0, 0.08)',
    },
    statCard: {
      boxShadow: '0 2px 4px rgba(0, 0, 0, 0.08)',
      emptyIconBg: 'rgba(245, 158, 11, 0.1)',
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
    axis: 'rgba(255, 255, 255, 0.15)',
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

  // Text colors - warm tones for sunlit theme
  const textPrimary = isSunlit ? '#292524' : (isDark ? '#f1f5f9' : '#1e293b');
  const textSecondary = isSunlit ? '#78716c' : (isDark ? '#94a3b8' : '#64748b');
  const textDisabled = isSunlit ? '#a8a29e' : (isDark ? '#64748b' : '#94a3b8');

  const dividerColor = isSunlit ? 'rgba(0, 0, 0, 0.06)' : (isDark ? '#334155' : '#e2e8f0');

  // Dashboard-specific colors
  const dashboardColors = isSunlit
    ? SUNLIT_PALETTE.dashboard
    : (isDark ? DARK_DASHBOARD_COLORS : LIGHT_DASHBOARD_COLORS);

  return {
    palette: {
      mode: isSunlit ? 'light' : mode,
      primary: primaryColor,
      secondary: secondaryColor,
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
      background: backgroundColor,
      text: {
        primary: textPrimary,
        secondary: textSecondary,
        disabled: textDisabled,
      },
      divider: dividerColor,
      action: {
        hover: isSunlit ? 'rgba(0, 0, 0, 0.04)' : (isDark ? '#1e293b' : '#f1f5f9'),
        selected: isSunlit ? 'rgba(217, 119, 6, 0.12)' : (isDark ? '#1e3a8a' : '#e0e7ff'),
        disabled: isSunlit ? 'rgba(0, 0, 0, 0.04)' : (isDark ? '#1e293b' : '#f1f5f9'),
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
              ? '0 4px 16px rgba(245, 158, 11, 0.15), 0 2px 6px rgba(0, 0, 0, 0.08)'
              : (isDark
                ? '0 1px 3px 0 rgba(0, 0, 0, 0.3), 0 1px 2px 0 rgba(0, 0, 0, 0.2)'
                : '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'),
            borderRadius: 12,
            border: isSunlit
              ? '1px solid rgba(245, 158, 11, 0.2)'
              : (isDark ? '1px solid #334155' : '1px solid #e2e8f0'),
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.75)'
              : (isDark ? '#1e293b' : '#ffffff'),
            backdropFilter: isSunlit ? 'blur(12px)' : 'none',
          },
        },
      },
      MuiListItemButton: {
        styleOverrides: {
          root: {
            '&.nav-item-active': {
              backgroundColor: isSunlit ? '#f59e0b' : '#2563eb',
              color: '#ffffff',
              '&:hover': {
                backgroundColor: isSunlit ? '#d97706' : '#1d4ed8',
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
                ? '0 2px 4px rgba(0, 0, 0, 0.1)'
                : (isDark
                  ? '0 1px 2px 0 rgba(0, 0, 0, 0.3)'
                  : '0 1px 2px 0 rgba(0, 0, 0, 0.05)'),
            },
          },
          contained: {
            background: isSunlit
              ? 'linear-gradient(135deg, #f59e0b 0%, #d97706 100%)'
              : 'linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%)',
            '&:hover': {
              background: isSunlit
                ? 'linear-gradient(135deg, #d97706 0%, #b45309 100%)'
                : 'linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%)',
            },
          },
          outlined: {
            borderColor: isSunlit ? 'rgba(0, 0, 0, 0.15)' : (isDark ? '#475569' : '#d1d5db'),
            color: isSunlit ? '#1c1917' : (isDark ? '#cbd5e1' : '#374151'),
            '&:hover': {
              borderColor: isSunlit ? 'rgba(0, 0, 0, 0.25)' : (isDark ? '#64748b' : '#9ca3af'),
              backgroundColor: isSunlit ? 'rgba(0, 0, 0, 0.04)' : (isDark ? '#334155' : '#f9fafb'),
            },
          },
        },
      },
      MuiTextField: {
        styleOverrides: {
          root: {
            '& .MuiOutlinedInput-root': {
              borderRadius: 6,
              backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.5)' : 'transparent',
              '& fieldset': {
                borderColor: isSunlit ? 'rgba(0, 0, 0, 0.15)' : (isDark ? '#475569' : '#d1d5db'),
              },
              '&:hover fieldset': {
                borderColor: isSunlit ? 'rgba(0, 0, 0, 0.25)' : (isDark ? '#64748b' : '#9ca3af'),
              },
              '&.Mui-focused fieldset': {
                borderColor: isSunlit ? '#f59e0b' : '#2563eb',
                borderWidth: 1,
              },
            },
          },
        },
      },
      MuiSelect: {
        styleOverrides: {
          root: {
            '& .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? 'rgba(0, 0, 0, 0.15)' : (isDark ? '#475569' : '#d1d5db'),
            },
            '&:hover .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? 'rgba(0, 0, 0, 0.25)' : (isDark ? '#64748b' : '#9ca3af'),
            },
            '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? '#f59e0b' : '#2563eb',
              borderWidth: 1,
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
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.9)' : undefined,
          },
        },
      },
      MuiDrawer: {
        styleOverrides: {
          paper: {
            borderRight: isSunlit ? '1px solid rgba(0, 0, 0, 0.08)' : (isDark ? '1px solid #334155' : '1px solid #e2e8f0'),
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.7)' : undefined,
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
            backgroundColor: isSunlit ? '#f59e0b' : '#2563eb',
          },
        },
      },
      MuiPaper: {
        styleOverrides: {
          root: {
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.6)' : undefined,
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
