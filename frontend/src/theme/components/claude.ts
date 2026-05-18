import type { ThemeOptions } from '@mui/material/styles';

// Anthropic brand tokens: Dark #141413 · Light #FAF9F5 · Light Gray #E8E6DC · Orange #D97757
export const claudeComponents: ThemeOptions['components'] = {
  MuiCard: {
    styleOverrides: {
      root: {
        boxShadow: '0 1px 3px 0 rgba(20, 20, 19, 0.08), 0 1px 2px 0 rgba(20, 20, 19, 0.04)',
        borderRadius: 12,
        border: '1px solid #E8E6DC',
        backgroundColor: '#FFFFFF',
        backgroundImage: 'none',
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: {
        '&.nav-item-active': {
          backgroundColor: '#D97757',
          color: '#ffffff',
          '&:hover': { backgroundColor: '#C26146' },
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
          boxShadow: '0 1px 2px 0 rgba(20, 20, 19, 0.06)',
        },
      },
      contained: {
        background: 'linear-gradient(135deg, #D97757 0%, #C26146 100%)',
        '&:hover': {
          background: 'linear-gradient(135deg, #C26146 0%, #A85138 100%)',
        },
      },
      outlined: {
        borderColor: '#D6D3C7',
        color: '#141413',
        '&:hover': {
          borderColor: '#B0AEA5',
          backgroundColor: '#F2EFE3',
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
        '& .MuiOutlinedInput-notchedOutline': { borderColor: '#D6D3C7' },
        '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#B0AEA5' },
        '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
          borderColor: '#D97757',
          borderWidth: 1.5,
        },
        '&.Mui-disabled .MuiOutlinedInput-notchedOutline': {
          borderColor: '#E8E6DC',
        },
        '&.Mui-error .MuiOutlinedInput-notchedOutline': {
          borderColor: '#BF4434',
        },
      },
      input: {
        '&::placeholder': { color: '#B0AEA5', opacity: 1 },
      },
    },
  },
  MuiFilledInput: {
    styleOverrides: {
      root: {
        backgroundColor: 'rgba(20, 20, 19, 0.04)',
        '&:hover': { backgroundColor: 'rgba(20, 20, 19, 0.06)' },
        '&.Mui-focused': { backgroundColor: 'rgba(20, 20, 19, 0.06)' },
      },
    },
  },
  MuiInputBase: {
    styleOverrides: {
      input: {
        color: '#141413',
        '&::placeholder': { color: '#B0AEA5', opacity: 1 },
      },
    },
  },
  MuiInputLabel: {
    styleOverrides: {
      root: {
        color: '#6B6863',
        '&.Mui-focused': { color: '#D97757' },
      },
    },
  },
  MuiFormHelperText: {
    styleOverrides: {
      root: {
        color: '#6B6863',
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
        border: '1px solid #E8E6DC',
        boxShadow: '0 10px 24px rgba(20, 20, 19, 0.08)',
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        '&:hover': { backgroundColor: '#F2EFE3' },
        '&.Mui-selected': {
          backgroundColor: '#E8E6DC',
          '&:hover': { backgroundColor: '#DCD9CB' },
        },
      },
    },
  },
  MuiDrawer: {
    styleOverrides: {
      paper: {
        borderRight: '1px solid #E8E6DC',
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
        color: '#141413',
        borderBottom: '1px solid #E8E6DC',
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
        backgroundColor: 'rgba(20, 20, 19, 0.92)',
        color: '#FAF9F5',
        fontSize: '0.75rem',
        border: 'none',
      },
      arrow: { color: 'rgba(20, 20, 19, 0.92)' },
    },
  },
  MuiDivider: {
    styleOverrides: {
      root: { borderColor: '#E8E6DC' },
    },
  },
  MuiTabs: {
    styleOverrides: {
      indicator: {
        height: 4,
        borderRadius: 2,
        backgroundColor: '#D97757',
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
