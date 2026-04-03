import { createTheme, type ThemeOptions } from '@mui/material/styles';

// Sunlit theme palette - warm, natural tones that complement the blinds effect
const SUNLIT_PALETTE = {
  primary: {
    main: '#d97706', // Warm amber/orange for sunlit theme
    light: '#f59e0b',
    dark: '#b45309',
    contrastText: '#ffffff',
  },
  secondary: {
    main: '#78716c', // Warm stone gray
    light: '#a8a29e',
    dark: '#57534e',
    contrastText: '#ffffff',
  },
  background: {
    default: 'transparent',
    paper: 'rgba(255, 255, 255, 0.6)', // Semi-transparent white
    paperSolid: 'rgba(255, 255, 255, 0.85)', // Less transparent for important cards
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

  // Text colors - darker for sunlit theme to be readable on light backgrounds
  const textPrimary = isSunlit ? '#1c1917' : (isDark ? '#f1f5f9' : '#1e293b');
  const textSecondary = isSunlit ? '#57534e' : (isDark ? '#94a3b8' : '#64748b');
  const textDisabled = isSunlit ? '#a8a29e' : (isDark ? '#64748b' : '#94a3b8');

  const dividerColor = isSunlit ? 'rgba(0, 0, 0, 0.08)' : (isDark ? '#334155' : '#e2e8f0');

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
              ? '0 2px 8px rgba(0, 0, 0, 0.08), 0 1px 3px rgba(0, 0, 0, 0.05)'
              : (isDark
                ? '0 1px 3px 0 rgba(0, 0, 0, 0.3), 0 1px 2px 0 rgba(0, 0, 0, 0.2)'
                : '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'),
            borderRadius: 8,
            border: isSunlit
              ? '1px solid rgba(255, 255, 255, 0.3)'
              : (isDark ? '1px solid #334155' : '1px solid #e2e8f0'),
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.65)'
              : (isDark ? '#1e293b' : '#ffffff'),
            backdropFilter: isSunlit ? 'blur(12px)' : 'none',
          },
        },
      },
      MuiListItemButton: {
        styleOverrides: {
          root: {
            '&.nav-item-active': {
              backgroundColor: isSunlit ? '#d97706' : '#2563eb',
              color: '#ffffff',
              '&:hover': {
                backgroundColor: isSunlit ? '#b45309' : '#1d4ed8',
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
              ? 'linear-gradient(135deg, #d97706 0%, #b45309 100%)'
              : 'linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%)',
            '&:hover': {
              background: isSunlit
                ? 'linear-gradient(135deg, #b45309 0%, #92400e 100%)'
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
                borderColor: isSunlit ? '#d97706' : '#2563eb',
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
              borderColor: isSunlit ? '#d97706' : '#2563eb',
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
            backdropFilter: isSunlit ? 'blur(12px)' : 'none',
          },
        },
      },
      MuiTabs: {
        styleOverrides: {
          indicator: {
            height: 4,
            borderRadius: 2,
            backgroundColor: isSunlit ? '#d97706' : '#2563eb',
          },
        },
      },
      MuiPaper: {
        styleOverrides: {
          root: {
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.6)' : undefined,
            backdropFilter: isSunlit ? 'blur(12px)' : 'none',
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
