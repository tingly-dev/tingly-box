import type { ThemeOptions } from '@mui/material/styles';

export const claudeComponents: ThemeOptions['components'] = {
  MuiCard: {
    styleOverrides: {
      root: {
        boxShadow: '0 1px 3px 0 rgba(31, 30, 29, 0.08), 0 1px 2px 0 rgba(31, 30, 29, 0.04)',
        borderRadius: 12,
        border: '1px solid #E8E4DA',
        backgroundColor: '#FFFFFF',
        backgroundImage: 'none',
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: {
        '&.nav-item-active': {
          backgroundColor: '#C96442',
          color: '#ffffff',
          '&:hover': { backgroundColor: '#B8552F' },
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
          boxShadow: '0 1px 2px 0 rgba(31, 30, 29, 0.06)',
        },
      },
      contained: {
        background: 'linear-gradient(135deg, #C96442 0%, #B8552F 100%)',
        '&:hover': {
          background: 'linear-gradient(135deg, #B8552F 0%, #9A4622 100%)',
        },
      },
      outlined: {
        borderColor: '#D6CFC1',
        color: '#3A3733',
        '&:hover': {
          borderColor: '#B8AE9B',
          backgroundColor: '#F5F1E6',
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
        '& .MuiOutlinedInput-notchedOutline': { borderColor: '#D6CFC1' },
        '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#B8AE9B' },
        '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
          borderColor: '#C96442',
          borderWidth: 1.5,
        },
        '&.Mui-disabled .MuiOutlinedInput-notchedOutline': {
          borderColor: '#E8E4DA',
        },
        '&.Mui-error .MuiOutlinedInput-notchedOutline': {
          borderColor: '#BF4434',
        },
      },
      input: {
        '&::placeholder': { color: '#A8A39B', opacity: 1 },
      },
    },
  },
  MuiFilledInput: {
    styleOverrides: {
      root: {
        backgroundColor: 'rgba(31, 30, 29, 0.04)',
        '&:hover': { backgroundColor: 'rgba(31, 30, 29, 0.06)' },
        '&.Mui-focused': { backgroundColor: 'rgba(31, 30, 29, 0.06)' },
      },
    },
  },
  MuiInputBase: {
    styleOverrides: {
      input: {
        color: '#1F1E1D',
        '&::placeholder': { color: '#A8A39B', opacity: 1 },
      },
    },
  },
  MuiInputLabel: {
    styleOverrides: {
      root: {
        color: '#6B6863',
        '&.Mui-focused': { color: '#C96442' },
      },
    },
  },
  MuiFormHelperText: {
    styleOverrides: {
      root: {
        color: '#7A766F',
        '&.Mui-error': { color: '#BF4434' },
      },
    },
  },
  MuiSelect: {
    styleOverrides: {
      icon: { color: '#6B6863' },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        backgroundColor: '#FFFFFF',
        backgroundImage: 'none',
        border: '1px solid #E8E4DA',
        boxShadow: '0 10px 24px rgba(31, 30, 29, 0.08)',
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        '&:hover': { backgroundColor: '#F2EEE3' },
        '&.Mui-selected': {
          backgroundColor: '#EAE0CF',
          '&:hover': { backgroundColor: '#DFD2BC' },
        },
      },
    },
  },
  MuiDrawer: {
    styleOverrides: {
      paper: {
        borderRight: '1px solid #E8E4DA',
        backgroundImage: 'none',
        willChange: 'auto',
      },
    },
  },
  MuiAppBar: {
    styleOverrides: {
      root: {
        backgroundColor: '#FAF9F5',
        backgroundImage: 'none',
        color: '#1F1E1D',
        borderBottom: '1px solid #E8E4DA',
        boxShadow: 'none',
      },
    },
  },
  MuiDialog: {
    styleOverrides: {
      paper: {
        backgroundColor: '#FFFFFF',
        backgroundImage: 'none',
      },
    },
  },
  MuiPopover: {
    styleOverrides: {
      paper: {
        backgroundColor: '#FFFFFF',
        backgroundImage: 'none',
      },
    },
  },
  MuiTooltip: {
    styleOverrides: {
      tooltip: {
        backgroundColor: 'rgba(31, 30, 29, 0.92)',
        color: '#FAF9F5',
        fontSize: '0.75rem',
        border: 'none',
      },
      arrow: { color: 'rgba(31, 30, 29, 0.92)' },
    },
  },
  MuiDivider: {
    styleOverrides: {
      root: { borderColor: '#E8E4DA' },
    },
  },
  MuiTabs: {
    styleOverrides: {
      indicator: {
        height: 4,
        borderRadius: 2,
        backgroundColor: '#C96442',
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
