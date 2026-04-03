import { createTheme, type ThemeOptions } from '@mui/material/styles';

// Sunlit theme palette - warm, sunny tones with natural elegance
// Inspired by golden hour sunlight, fresh foliage, and warm earth
const SUNLIT_PALETTE = {
  primary: {
    main: '#d97706', // Warm amber golden - more elegant
    light: '#f59e0b',
    dark: '#b45309',
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
    paper: 'rgba(255, 255, 255, 0.75)',
    paperSolid: 'rgba(255, 255, 255, 0.92)',
  },
  // Dashboard token colors for sunlit theme - natural elements
  dashboard: {
    token: {
      input: {
        main: '#0ea5e9', // Sky blue - softer, natural
        gradient: 'rgba(14, 165, 233, 0.75)',
      },
      output: {
        main: '#84cc16', // Fresh lime green - vibrant but natural
        gradient: 'rgba(132, 204, 22, 0.75)',
      },
      cache: {
        main: '#a78b71', // Earthy brown - warm, natural soil tone
        gradient: 'rgba(167, 139, 113, 0.65)',
      },
    },
    chart: {
      grid: 'rgba(0, 0, 0, 0.05)',
      axis: 'rgba(0, 0, 0, 0.08)',
      tooltipBg: 'rgba(255, 255, 255, 0.96)',
      tooltipBorder: 'rgba(217, 119, 6, 0.15)',
    },
    statCard: {
      boxShadow: '0 2px 8px rgba(217, 119, 6, 0.08), 0 1px 3px rgba(0, 0, 0, 0.05)',
      emptyIconBg: 'rgba(217, 119, 6, 0.08)',
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

  // Text colors - warm, natural tones for sunlit theme
  const textPrimary = isSunlit ? '#1c1917' : (isDark ? '#f8fafc' : '#1e293b');
  const textSecondary = isSunlit ? '#57534e' : (isDark ? '#cbd5e1' : '#64748b');
  const textDisabled = isSunlit ? '#a8a29e' : (isDark ? '#94a3b8' : '#94a3b8');

  const dividerColor = isSunlit ? 'rgba(0, 0, 0, 0.06)' : (isDark ? '#334155' : '#e2e8f0');

  // Dashboard-specific colors
  const dashboardColors = isSunlit
    ? SUNLIT_PALETTE.dashboard
    : (isDark ? DARK_DASHBOARD_COLORS : LIGHT_DASHBOARD_COLORS);

  // Common colors for sunlit theme
  const sunlitPrimary = '#d97706';
  const sunlitPrimaryLight = '#f59e0b';
  const sunlitPrimaryDark = '#b45309';

  return {
    palette: {
      mode: isSunlit ? 'light' : mode,
      primary: primaryColor,
      secondary: secondaryColor,
      success: {
        main: isSunlit ? '#16a34a' : '#059669',
        light: isSunlit ? '#22c55e' : '#10b981',
        dark: isSunlit ? '#15803d' : '#047857',
      },
      error: {
        main: isSunlit ? '#dc2626' : '#dc2626',
        light: isSunlit ? '#ef4444' : '#ef4444',
        dark: isSunlit ? '#b91c1c' : '#b91c1c',
      },
      warning: {
        main: isSunlit ? '#ea580c' : '#d97706',
        light: isSunlit ? '#f97316' : '#f59e0b',
        dark: isSunlit ? '#c2410c' : '#b45309',
      },
      info: {
        main: isSunlit ? '#0284c7' : '#0891b2',
      },
      background: backgroundColor,
      text: {
        primary: textPrimary,
        secondary: textSecondary,
        disabled: textDisabled,
      },
      divider: dividerColor,
      action: {
        hover: isSunlit ? 'rgba(217, 119, 6, 0.06)' : (isDark ? '#1e293b' : '#f1f5f9'),
        selected: isSunlit ? 'rgba(217, 119, 6, 0.12)' : (isDark ? '#1e3a8a' : '#e0e7ff'),
        disabled: isSunlit ? 'rgba(217, 119, 6, 0.04)' : (isDark ? '#1e293b' : '#f1f5f9'),
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
              ? '0 2px 12px rgba(217, 119, 6, 0.08), 0 1px 4px rgba(0, 0, 0, 0.04)'
              : (isDark
                ? '0 1px 3px 0 rgba(0, 0, 0, 0.3), 0 1px 2px 0 rgba(0, 0, 0, 0.2)'
                : '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'),
            borderRadius: 12,
            border: isSunlit
              ? '1px solid rgba(217, 119, 6, 0.12)'
              : (isDark ? '1px solid #334155' : '1px solid #e2e8f0'),
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.8)'
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
                ? '0 2px 6px rgba(217, 119, 6, 0.15)'
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
                ? `linear-gradient(135deg, ${sunlitPrimaryDark} 0%, #92400e 100%)`
                : 'linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%)',
            },
          },
          outlined: {
            borderColor: isSunlit ? 'rgba(217, 119, 6, 0.25)' : (isDark ? '#475569' : '#d1d5db'),
            color: isSunlit ? sunlitPrimaryDark : (isDark ? '#cbd5e1' : '#374151'),
            '&:hover': {
              borderColor: isSunlit ? 'rgba(217, 119, 6, 0.4)' : (isDark ? '#64748b' : '#9ca3af'),
              backgroundColor: isSunlit ? 'rgba(217, 119, 6, 0.08)' : (isDark ? '#334155' : '#f9fafb'),
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
                borderColor: isSunlit ? 'rgba(217, 119, 6, 0.2)' : (isDark ? '#475569' : '#d1d5db'),
              },
              '&:hover fieldset': {
                borderColor: isSunlit ? 'rgba(217, 119, 6, 0.35)' : (isDark ? '#64748b' : '#9ca3af'),
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
              borderColor: isSunlit ? 'rgba(217, 119, 6, 0.2)' : (isDark ? '#475569' : '#d1d5db'),
            },
            '&:hover .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? 'rgba(217, 119, 6, 0.35)' : (isDark ? '#64748b' : '#9ca3af'),
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
            borderRight: isSunlit ? '1px solid rgba(217, 119, 6, 0.15)' : (isDark ? '1px solid #334155' : '1px solid #e2e8f0'),
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
