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
    grid: 'rgba(255, 255, 255, 0.06)',
    axis: 'rgba(255, 255, 255, 0.18)',
    tooltipBg: '#181c26',
    tooltipBorder: 'rgba(255, 255, 255, 0.12)',
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

  // Primary blue: dark mode shifts one notch up (blue-500 instead of
  // blue-600) so text-variant Buttons reach WCAG AA against Paper while
  // staying visually aligned with the contained-Button gradient
  // (#3b82f6 → #2563eb) — i.e. selected ToggleButton, Tabs indicator,
  // and nav-active surfaces sit on the same blue ramp instead of a
  // paler off-shade.
  const primaryColor = isSunlit ? SUNLIT_PALETTE.primary : {
    main: isDark ? '#3b82f6' : '#2563eb',
    light: isDark ? '#60a5fa' : '#3b82f6',
    dark: isDark ? '#2563eb' : '#1d4ed8',
    contrastText: '#ffffff',
  };

  const secondaryColor = isSunlit ? SUNLIT_PALETTE.secondary : {
    main: isDark ? '#94a3b8' : '#64748b',
    light: '#cbd5e1',
    dark: '#475569',
    contrastText: '#ffffff',
  };

  // Dark-mode surface ladder — aligned with MUI's official dark theme
  // (~7% / 12% lightness) but keeps a subtle slate hue instead of pure neutral
  // gray. The previous slate-900/800 pair sat at ~11% / 17% lightness, which
  // is why it read as "dim gray-blue" rather than truly dark.
  //   default  #07090f  ~3.5% L  — app background (true near-black)
  //   paper    #11141c  ~10%  L  — cards, dialogs, dense surfaces
  //   raised   #181c26  ~13%  L  — menus, drawers, popovers (one step up)
  const darkBgDefault = '#07090f';
  const darkBgPaper = '#11141c';
  const darkBgRaised = '#181c26';

  const backgroundColor = isSunlit ? SUNLIT_PALETTE.background : {
    default: isDark ? darkBgDefault : '#f8fafc',
    paper: isDark ? darkBgPaper : '#ffffff',
  };

  // Text colors - clear and fresh for sky blue theme
  const textPrimary = isSunlit ? '#0f172a' : (isDark ? '#f3f5f9' : '#1e293b');
  const textSecondary = isSunlit ? '#475569' : (isDark ? 'rgba(255, 255, 255, 0.72)' : '#64748b');
  const textDisabled = isSunlit ? '#94a3b8' : (isDark ? 'rgba(255, 255, 255, 0.42)' : '#94a3b8');

  const dividerColor = isSunlit
    ? 'rgba(14, 165, 233, 0.12)'
    : (isDark ? 'rgba(255, 255, 255, 0.12)' : '#e2e8f0');

  // Dark-mode input surface tokens — sit one elevation step above Paper so
  // fields are recognizable without relying on the border alone.
  const darkInputBg = 'rgba(255, 255, 255, 0.05)';
  const darkInputBgHover = 'rgba(255, 255, 255, 0.08)';
  const darkInputBorder = 'rgba(255, 255, 255, 0.18)';
  const darkInputBorderHover = 'rgba(255, 255, 255, 0.32)';

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
      success: {
        main: isSunlit ? '#22c55e' : '#059669',
        light: isSunlit ? '#4ade80' : '#10b981',
        dark: isSunlit ? '#16a34a' : '#047857',
      },
      error: {
        // Dark mode: #dc2626 only reaches ~3.8:1 on Paper #11141c. Use the
        // lighter shade (#ef4444 → ~4.9:1) so Delete buttons in dialogs and
        // error helper text meet WCAG AA.
        main: isSunlit ? '#ef4444' : (isDark ? '#ef4444' : '#dc2626'),
        light: isSunlit ? '#f87171' : (isDark ? '#f87171' : '#ef4444'),
        dark: isSunlit ? '#dc2626' : (isDark ? '#dc2626' : '#b91c1c'),
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
        hover: isSunlit ? 'rgba(14, 165, 233, 0.08)' : (isDark ? 'rgba(255, 255, 255, 0.08)' : '#f1f5f9'),
        selected: isSunlit ? 'rgba(14, 165, 233, 0.15)' : (isDark ? 'rgba(59, 130, 246, 0.28)' : '#e0e7ff'),
        disabled: isSunlit ? 'rgba(14, 165, 233, 0.04)' : (isDark ? 'rgba(255, 255, 255, 0.05)' : '#f1f5f9'),
        focus: isSunlit ? 'rgba(14, 165, 233, 0.12)' : (isDark ? 'rgba(255, 255, 255, 0.12)' : '#e0e7ff'),
      },
      // Dashboard colors palette - custom extension
      // @ts-ignore - custom dashboard colors
      dashboard: {
        token: dashboardColors.token,
        chart: dashboardColors.chart,
        statCard: dashboardColors.statCard,
      } as any,
      // Theme mode flag for easy detection in components
      // @ts-ignore - custom theme flag
      isSunlit: isSunlit,
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
                ? '0 1px 2px 0 rgba(0, 0, 0, 0.6), 0 4px 12px 0 rgba(0, 0, 0, 0.3)'
                : '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'),
            borderRadius: 12,
            border: isSunlit
              ? '1px solid rgba(14, 165, 233, 0.15)'
              : (isDark ? '1px solid rgba(255, 255, 255, 0.08)' : '1px solid #e2e8f0'),
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.82)'
              : (isDark ? darkBgPaper : '#ffffff'),
            backgroundImage: 'none',
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
              : (isDark
                ? 'linear-gradient(135deg, #3b82f6 0%, #2563eb 100%)'
                : 'linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%)'),
            '&:hover': {
              background: isSunlit
                ? `linear-gradient(135deg, ${sunlitPrimaryDark} 0%, #0369a1 100%)`
                : (isDark
                  ? 'linear-gradient(135deg, #60a5fa 0%, #3b82f6 100%)'
                  : 'linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%)'),
            },
          },
          outlined: {
            borderColor: isSunlit ? 'rgba(14, 165, 233, 0.3)' : (isDark ? 'rgba(255, 255, 255, 0.23)' : '#d1d5db'),
            color: isSunlit ? '#0369a1' : (isDark ? '#e2e8f0' : '#374151'),
            '&:hover': {
              borderColor: isSunlit ? 'rgba(14, 165, 233, 0.5)' : (isDark ? 'rgba(255, 255, 255, 0.4)' : '#9ca3af'),
              backgroundColor: isSunlit ? 'rgba(14, 165, 233, 0.08)' : (isDark ? 'rgba(255, 255, 255, 0.08)' : '#f9fafb'),
            },
          },
        },
      },
      MuiOutlinedInput: {
        styleOverrides: {
          root: {
            borderRadius: 6,
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.6)'
              : (isDark ? darkInputBg : 'transparent'),
            transition: 'background-color 120ms ease, border-color 120ms ease',
            '& .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? 'rgba(14, 165, 233, 0.25)' : (isDark ? darkInputBorder : '#d1d5db'),
            },
            '&:hover': {
              backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.72)' : (isDark ? darkInputBgHover : 'transparent'),
            },
            '&:hover .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? 'rgba(14, 165, 233, 0.4)' : (isDark ? darkInputBorderHover : '#9ca3af'),
            },
            '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? sunlitPrimary : (isDark ? '#3b82f6' : '#2563eb'),
              borderWidth: 1.5,
            },
            '&.Mui-disabled': {
              backgroundColor: isSunlit
                ? 'rgba(255, 255, 255, 0.4)'
                : (isDark ? 'rgba(255, 255, 255, 0.02)' : 'transparent'),
              '& .MuiOutlinedInput-notchedOutline': {
                borderColor: isSunlit ? 'rgba(14, 165, 233, 0.12)' : (isDark ? 'rgba(148, 163, 184, 0.18)' : '#e5e7eb'),
              },
            },
            '&.Mui-error .MuiOutlinedInput-notchedOutline': {
              borderColor: isSunlit ? '#ef4444' : (isDark ? '#f87171' : '#dc2626'),
            },
          },
          input: {
            '&::placeholder': {
              color: isSunlit ? '#64748b' : (isDark ? '#94a3b8' : '#94a3b8'),
              opacity: 1,
            },
          },
        },
      },
      MuiFilledInput: {
        styleOverrides: {
          root: {
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.6)'
              : (isDark ? darkInputBg : 'rgba(0, 0, 0, 0.04)'),
            '&:hover': {
              backgroundColor: isSunlit
                ? 'rgba(255, 255, 255, 0.72)'
                : (isDark ? darkInputBgHover : 'rgba(0, 0, 0, 0.06)'),
            },
            '&.Mui-focused': {
              backgroundColor: isSunlit
                ? 'rgba(255, 255, 255, 0.82)'
                : (isDark ? darkInputBgHover : 'rgba(0, 0, 0, 0.06)'),
            },
          },
        },
      },
      MuiInputBase: {
        styleOverrides: {
          input: {
            color: textPrimary,
            '&::placeholder': {
              color: isSunlit ? '#64748b' : (isDark ? '#94a3b8' : '#94a3b8'),
              opacity: 1,
            },
          },
        },
      },
      MuiInputLabel: {
        styleOverrides: {
          root: {
            color: isSunlit ? '#475569' : (isDark ? '#cbd5e1' : '#64748b'),
            '&.Mui-focused': {
              color: isSunlit ? sunlitPrimary : (isDark ? '#3b82f6' : '#2563eb'),
            },
          },
        },
      },
      MuiFormHelperText: {
        styleOverrides: {
          root: {
            color: isSunlit ? '#64748b' : (isDark ? '#94a3b8' : '#6b7280'),
            '&.Mui-error': {
              color: isSunlit ? '#ef4444' : (isDark ? '#f87171' : '#dc2626'),
            },
          },
        },
      },
      MuiSelect: {
        styleOverrides: {
          icon: {
            color: isSunlit ? '#475569' : (isDark ? '#cbd5e1' : '#64748b'),
          },
        },
      },
      MuiMenu: {
        styleOverrides: {
          paper: {
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.96)' : (isDark ? darkBgRaised : '#ffffff'),
            backgroundImage: 'none',
            border: isSunlit
              ? '1px solid rgba(14, 165, 233, 0.15)'
              : (isDark ? '1px solid rgba(255, 255, 255, 0.1)' : '1px solid #e2e8f0'),
            boxShadow: isDark
              ? '0 10px 32px rgba(0, 0, 0, 0.7), 0 2px 8px rgba(0, 0, 0, 0.5)'
              : '0 10px 24px rgba(15, 23, 42, 0.08)',
          },
        },
      },
      MuiMenuItem: {
        styleOverrides: {
          root: {
            '&:hover': {
              backgroundColor: isSunlit
                ? 'rgba(14, 165, 233, 0.08)'
                : (isDark ? 'rgba(255, 255, 255, 0.08)' : '#f1f5f9'),
            },
            '&.Mui-selected': {
              backgroundColor: isSunlit
                ? 'rgba(14, 165, 233, 0.16)'
                : (isDark ? 'rgba(59, 130, 246, 0.24)' : '#e0e7ff'),
              '&:hover': {
                backgroundColor: isSunlit
                  ? 'rgba(14, 165, 233, 0.22)'
                  : (isDark ? 'rgba(59, 130, 246, 0.32)' : '#c7d2fe'),
              },
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
            borderRight: isSunlit
              ? '1px solid rgba(14, 165, 233, 0.15)'
              : (isDark ? '1px solid rgba(255, 255, 255, 0.08)' : '1px solid #e2e8f0'),
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.72)'
              : (isDark ? darkBgPaper : undefined),
            backgroundImage: 'none',
            backdropFilter: isSunlit ? 'blur(8px)' : 'none',
            willChange: 'auto',
          },
        },
      },
      MuiAppBar: {
        styleOverrides: {
          root: {
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.72)'
              : (isDark ? darkBgPaper : '#ffffff'),
            backgroundImage: 'none',
            color: textPrimary,
            borderBottom: isSunlit
              ? '1px solid rgba(14, 165, 233, 0.15)'
              : (isDark ? '1px solid rgba(255, 255, 255, 0.08)' : '1px solid #e2e8f0'),
            boxShadow: 'none',
          },
        },
      },
      MuiDialog: {
        styleOverrides: {
          paper: {
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.92)'
              : (isDark ? darkBgRaised : '#ffffff'),
            backgroundImage: 'none',
            border: isDark && !isSunlit ? '1px solid rgba(255, 255, 255, 0.08)' : undefined,
          },
        },
      },
      MuiPopover: {
        styleOverrides: {
          paper: {
            backgroundColor: isSunlit
              ? 'rgba(255, 255, 255, 0.96)'
              : (isDark ? darkBgRaised : '#ffffff'),
            backgroundImage: 'none',
            border: isDark && !isSunlit ? '1px solid rgba(255, 255, 255, 0.1)' : undefined,
          },
        },
      },
      MuiTooltip: {
        styleOverrides: {
          tooltip: {
            backgroundColor: isDark ? 'rgba(40, 44, 56, 0.96)' : 'rgba(15, 23, 42, 0.92)',
            color: '#f8fafc',
            fontSize: '0.75rem',
            border: isDark ? '1px solid rgba(255, 255, 255, 0.1)' : 'none',
          },
          arrow: {
            color: isDark ? 'rgba(40, 44, 56, 0.96)' : 'rgba(15, 23, 42, 0.92)',
          },
        },
      },
      MuiDivider: {
        styleOverrides: {
          root: {
            borderColor: isSunlit
              ? 'rgba(14, 165, 233, 0.12)'
              : (isDark ? 'rgba(255, 255, 255, 0.1)' : '#e2e8f0'),
          },
        },
      },
      MuiTabs: {
        styleOverrides: {
          indicator: {
            height: 4,
            borderRadius: 2,
            backgroundColor: isSunlit ? sunlitPrimary : (isDark ? '#3b82f6' : '#2563eb'),
          },
        },
      },
      MuiPaper: {
        styleOverrides: {
          root: {
            backgroundColor: isSunlit ? 'rgba(255, 255, 255, 0.65)' : undefined,
            // MUI auto-lightens Paper at higher elevations via a white overlay
            // gradient. Disable it so our explicit dark surface ladder stays
            // consistent — elevation reads as shadow, not as a brighter fill.
            backgroundImage: isDark ? 'none' : undefined,
            backdropFilter: isSunlit ? 'blur(8px)' : 'none',
            willChange: 'auto',
          },
        },
      },
      MuiTableCell: {
        styleOverrides: {
          root: {
            borderBottom: isSunlit
              ? '1px solid rgba(14, 165, 233, 0.1)'
              : undefined,
          },
          head: {
            backgroundColor: isSunlit
              ? 'rgba(14, 165, 233, 0.06)'
              : (isDark ? 'rgba(255, 255, 255, 0.03)' : undefined),
            fontWeight: 600,
          },
        },
      },
      MuiTableRow: {
        styleOverrides: {
          root: {
            '&:hover': {
              backgroundColor: isSunlit
                ? 'rgba(14, 165, 233, 0.04)'
                : undefined,
            },
          },
        },
      },
      MuiSwitch: {
        styleOverrides: {
          switchBase: {
            '&.Mui-checked': {
              color: isSunlit ? sunlitPrimary : undefined,
              '& + .MuiSwitch-track': {
                backgroundColor: isSunlit ? sunlitPrimary : undefined,
                opacity: isSunlit ? 0.6 : undefined,
              },
            },
          },
          track: {
            backgroundColor: isSunlit ? 'rgba(14, 165, 233, 0.3)' : undefined,
          },
        },
      },
      MuiSlider: {
        styleOverrides: {
          root: {
            color: isSunlit ? sunlitPrimary : undefined,
          },
          thumb: {
            '&:hover, &.Mui-focusVisible': {
              boxShadow: isSunlit
                ? '0 0 0 8px rgba(14, 165, 233, 0.16)'
                : undefined,
            },
          },
          track: {
            background: isSunlit
              ? `linear-gradient(90deg, ${sunlitPrimaryLight} 0%, ${sunlitPrimary} 100%)`
              : undefined,
          },
        },
      },
      MuiLinearProgress: {
        styleOverrides: {
          root: {
            backgroundColor: isSunlit ? 'rgba(14, 165, 233, 0.15)' : undefined,
            borderRadius: 4,
          },
          bar: {
            background: isSunlit
              ? `linear-gradient(90deg, ${sunlitPrimaryLight} 0%, ${sunlitPrimary} 100%)`
              : undefined,
            borderRadius: 4,
          },
        },
      },
      MuiCircularProgress: {
        styleOverrides: {
          root: {
            color: isSunlit ? sunlitPrimary : undefined,
          },
        },
      },
      MuiToggleButton: {
        styleOverrides: {
          root: {
            borderColor: isSunlit ? 'rgba(14, 165, 233, 0.25)' : undefined,
            '&.Mui-selected': {
              backgroundColor: isSunlit ? 'rgba(14, 165, 233, 0.15)' : undefined,
              color: isSunlit ? sunlitPrimary : undefined,
              '&:hover': {
                backgroundColor: isSunlit ? 'rgba(14, 165, 233, 0.22)' : undefined,
              },
            },
          },
        },
      },
      MuiBadge: {
        styleOverrides: {
          badge: {
            background: isSunlit
              ? `linear-gradient(135deg, ${sunlitPrimary} 0%, ${sunlitPrimaryDark} 100%)`
              : undefined,
          },
        },
      },
      MuiSkeleton: {
        styleOverrides: {
          root: {
            backgroundColor: isSunlit
              ? 'rgba(14, 165, 233, 0.08)'
              : undefined,
          },
        },
      },
      MuiCssBaseline: {
        styleOverrides: {
          body: isSunlit ? {
            // Custom scrollbar for sunlit theme
            '&::-webkit-scrollbar': {
              width: 8,
              height: 8,
            },
            '&::-webkit-scrollbar-track': {
              backgroundColor: 'rgba(14, 165, 233, 0.05)',
              borderRadius: 4,
            },
            '&::-webkit-scrollbar-thumb': {
              backgroundColor: 'rgba(14, 165, 233, 0.25)',
              borderRadius: 4,
              '&:hover': {
                backgroundColor: 'rgba(14, 165, 233, 0.4)',
              },
            },
            '*::-webkit-scrollbar': {
              width: 6,
              height: 6,
            },
            '*::-webkit-scrollbar-track': {
              backgroundColor: 'rgba(14, 165, 233, 0.05)',
              borderRadius: 3,
            },
            '*::-webkit-scrollbar-thumb': {
              backgroundColor: 'rgba(14, 165, 233, 0.2)',
              borderRadius: 3,
              '&:hover': {
                backgroundColor: 'rgba(14, 165, 233, 0.35)',
              },
            },
          } : undefined,
        },
      },
    },
  };
};

const createAppTheme = (mode: 'light' | 'dark' | 'sunlit') => {
  return createTheme(getThemeOptions(mode));
};

export default createAppTheme;
