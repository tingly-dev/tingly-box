import { createTheme, type ThemeOptions } from '@mui/material/styles';

const getThemeOptions = (mode: 'light' | 'dark'): ThemeOptions => {
  const isDark = mode === 'dark';

  return {
    palette: {
      mode,
      primary: {
        main: '#2563eb',
        light: '#3b82f6',
        dark: '#1d4ed8',
        contrastText: '#ffffff',
      },
      secondary: {
        main: isDark ? '#94a3b8' : '#64748b',
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
        default: isDark ? '#0f172a' : '#f8fafc',
        paper: isDark ? '#1e293b' : '#ffffff',
      },
      text: {
        primary: isDark ? '#f1f5f9' : '#1e293b',
        secondary: isDark ? '#94a3b8' : '#64748b',
        disabled: isDark ? '#64748b' : '#94a3b8',
      },
      divider: isDark ? '#334155' : '#e2e8f0',
      action: {
        hover: isDark ? '#1e293b' : '#f1f5f9',
        selected: isDark ? '#1e3a8a' : '#e0e7ff',
        disabled: isDark ? '#1e293b' : '#f1f5f9',
      },
    },
    typography: {
      fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Helvetica", "Arial", sans-serif',
      h1: {
        fontSize: '2rem',
        fontWeight: 600,
        color: isDark ? '#f1f5f9' : '#1e293b',
      },
      h2: {
        fontSize: '1.5rem',
        fontWeight: 600,
        color: isDark ? '#f1f5f9' : '#1e293b',
      },
      h3: {
        fontSize: '1.25rem',
        fontWeight: 600,
        color: isDark ? '#f1f5f9' : '#1e293b',
      },
      h4: {
        fontSize: '1.125rem',
        fontWeight: 600,
        color: isDark ? '#f1f5f9' : '#1e293b',
      },
      h5: {
        fontSize: '1rem',
        fontWeight: 600,
        color: isDark ? '#f1f5f9' : '#1e293b',
      },
      h6: {
        fontSize: '0.875rem',
        fontWeight: 600,
        color: isDark ? '#f1f5f9' : '#1e293b',
      },
      body1: {
        fontSize: '0.875rem',
        color: isDark ? '#cbd5e1' : '#64748b',
      },
      body2: {
        fontSize: '0.75rem',
        color: isDark ? '#94a3b8' : '#64748b',
      },
      caption: {
        fontSize: '0.625rem',
        color: isDark ? '#64748b' : '#94a3b8',
      },
    },
    shape: {
      borderRadius: 8,
    },
    components: {
      MuiCard: {
        styleOverrides: {
          root: {
            boxShadow: isDark
              ? '0 1px 3px 0 rgba(0, 0, 0, 0.3), 0 1px 2px 0 rgba(0, 0, 0, 0.2)'
              : '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
            borderRadius: 8,
            border: isDark ? '1px solid #334155' : '1px solid #e2e8f0',
            backgroundColor: isDark ? '#1e293b' : '#ffffff',
          },
        },
      },
      MuiListItemButton: {
        styleOverrides: {
          root: {
            '&.nav-item-active': {
              backgroundColor: '#2563eb',
              color: '#ffffff',
              '&:hover': {
                backgroundColor: '#1d4ed8',
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
              boxShadow: isDark
                ? '0 1px 2px 0 rgba(0, 0, 0, 0.3)'
                : '0 1px 2px 0 rgba(0, 0, 0, 0.05)',
            },
          },
          contained: {
            background: 'linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%)',
            '&:hover': {
              background: 'linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%)',
            },
          },
          outlined: {
            borderColor: isDark ? '#475569' : '#d1d5db',
            color: isDark ? '#cbd5e1' : '#374151',
            '&:hover': {
              borderColor: isDark ? '#64748b' : '#9ca3af',
              backgroundColor: isDark ? '#334155' : '#f9fafb',
            },
          },
        },
      },
      MuiTextField: {
        styleOverrides: {
          root: {
            '& .MuiOutlinedInput-root': {
              borderRadius: 6,
              '& fieldset': {
                borderColor: isDark ? '#475569' : '#d1d5db',
              },
              '&:hover fieldset': {
                borderColor: isDark ? '#64748b' : '#9ca3af',
              },
              '&.Mui-focused fieldset': {
                borderColor: '#2563eb',
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
              borderColor: isDark ? '#475569' : '#d1d5db',
            },
            '&:hover .MuiOutlinedInput-notchedOutline': {
              borderColor: isDark ? '#64748b' : '#9ca3af',
            },
            '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
              borderColor: '#2563eb',
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
          },
        },
      },
      MuiDrawer: {
        styleOverrides: {
          paper: {
            borderRight: isDark ? '1px solid #334155' : '1px solid #e2e8f0',
          },
        },
      },
      MuiTabs: {
        styleOverrides: {
          indicator: {
            height: 4,
            borderRadius: 2,
            backgroundColor: '#2563eb',
          },
        },
      },
    },
  };
};

const createAppTheme = (mode: 'light' | 'dark') => {
  return createTheme(getThemeOptions(mode));
};

export default createAppTheme;
