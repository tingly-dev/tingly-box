import type { ThemeOptions } from '@mui/material/styles';

export const lightComponents: ThemeOptions['components'] = {
  MuiCard: {
    styleOverrides: {
      root: {
        boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
        borderRadius: 12,
        border: '1px solid #e2e8f0',
        backgroundColor: '#ffffff',
        backgroundImage: 'none',
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: {
        '&.nav-item-active': {
          backgroundColor: '#2563eb',
          color: '#ffffff',
          '&:hover': { backgroundColor: '#1d4ed8' },
          '& .MuiListItemIcon-root': { color: '#ffffff' },
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
          boxShadow: '0 1px 2px 0 rgba(0, 0, 0, 0.05)',
        },
      },
      contained: {
        background: 'linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%)',
        '&:hover': {
          background: 'linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%)',
        },
      },
      outlined: {
        borderColor: '#d1d5db',
        color: '#374151',
        '&:hover': {
          borderColor: '#9ca3af',
          backgroundColor: '#f9fafb',
        },
      },
    },
  },
  MuiOutlinedInput: {
    styleOverrides: {
      root: {
        borderRadius: 6,
        backgroundColor: 'transparent',
        transition: 'background-color 120ms ease, border-color 120ms ease',
        '& .MuiOutlinedInput-notchedOutline': { borderColor: '#d1d5db' },
        '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#9ca3af' },
        '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
          borderColor: '#2563eb',
          borderWidth: 1.5,
        },
        '&.Mui-disabled .MuiOutlinedInput-notchedOutline': {
          borderColor: '#e5e7eb',
        },
        '&.Mui-error .MuiOutlinedInput-notchedOutline': {
          borderColor: '#dc2626',
        },
      },
      input: {
        '&::placeholder': { color: '#94a3b8', opacity: 1 },
      },
    },
  },
  MuiFilledInput: {
    styleOverrides: {
      root: {
        backgroundColor: 'rgba(0, 0, 0, 0.04)',
        '&:hover': { backgroundColor: 'rgba(0, 0, 0, 0.06)' },
        '&.Mui-focused': { backgroundColor: 'rgba(0, 0, 0, 0.06)' },
      },
    },
  },
  MuiInputBase: {
    styleOverrides: {
      input: {
        color: '#1e293b',
        '&::placeholder': { color: '#94a3b8', opacity: 1 },
      },
    },
  },
  MuiInputLabel: {
    styleOverrides: {
      root: {
        color: '#64748b',
        '&.Mui-focused': { color: '#2563eb' },
      },
    },
  },
  MuiFormHelperText: {
    styleOverrides: {
      root: {
        color: '#6b7280',
        '&.Mui-error': { color: '#dc2626' },
      },
    },
  },
  MuiSelect: {
    styleOverrides: {
      icon: { color: '#64748b' },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        backgroundColor: '#ffffff',
        backgroundImage: 'none',
        border: '1px solid #e2e8f0',
        boxShadow: '0 10px 24px rgba(15, 23, 42, 0.08)',
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        '&:hover': { backgroundColor: '#f1f5f9' },
        '&.Mui-selected': {
          backgroundColor: '#e0e7ff',
          '&:hover': { backgroundColor: '#c7d2fe' },
        },
      },
    },
  },
  MuiDrawer: {
    styleOverrides: {
      paper: {
        borderRight: '1px solid #e2e8f0',
        backgroundImage: 'none',
        willChange: 'auto',
      },
    },
  },
  MuiAppBar: {
    styleOverrides: {
      root: {
        backgroundColor: '#ffffff',
        backgroundImage: 'none',
        color: '#1e293b',
        borderBottom: '1px solid #e2e8f0',
        boxShadow: 'none',
      },
    },
  },
  MuiDialog: {
    styleOverrides: {
      paper: {
        backgroundColor: '#ffffff',
        backgroundImage: 'none',
      },
    },
  },
  MuiPopover: {
    styleOverrides: {
      paper: {
        backgroundColor: '#ffffff',
        backgroundImage: 'none',
      },
    },
  },
  MuiTooltip: {
    styleOverrides: {
      tooltip: {
        backgroundColor: '#ffffff',
        color: '#1e293b',
        fontSize: '0.75rem',
        border: '1px solid #e2e8f0',
        boxShadow: '0 4px 12px rgba(15, 23, 42, 0.08)',
      },
      arrow: { color: '#ffffff' },
    },
  },
  MuiDivider: {
    styleOverrides: {
      root: { borderColor: '#e2e8f0' },
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
  MuiLinearProgress: {
    styleOverrides: {
      root: { borderRadius: 4 },
      bar: { borderRadius: 4 },
    },
  },
};
