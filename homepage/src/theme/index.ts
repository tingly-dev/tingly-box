import { createTheme } from "@mui/material/styles";

// Theme colors extracted from CSS variables and custom colors
const themeColors = {
  // Background colors - transparent for animated background
  background: 'transparent',
  foreground: 'hsl(210, 40%, 98%)',

  // Card colors - more transparent with subtle backdrop
  card: 'rgba(15, 15, 25, 0.7)',
  cardForeground: 'hsl(210, 40%, 98%)',

  // Popover colors - transparent
  popover: 'rgba(15, 15, 25, 0.8)',
  popoverForeground: 'hsl(210, 40%, 98%)',

  // Primary colors - using custom blue colors
  primary: {
    main: '#e8f4ff', // Light blue
    light: '#ffffff',
    dark: '#b8d8f8',
    contrastText: '#0f172a', // Dark text for contrast
  },

  // Secondary colors
  secondary: {
    main: '#b8d8f8', // Medium blue
    light: '#d4e4f7',
    dark: '#9cc5f3',
    contrastText: '#0f172a',
  },

  // Accent colors - using pink
  accent: {
    main: '#ffc8dd', // Light pink
    contrastText: '#0f172a',
  },

  // Muted colors
  muted: 'hsl(220, 15%, 15%)',
  mutedForeground: 'hsl(215, 20%, 55%)',

  // Destructive colors
  destructive: {
    main: '#ff4757',
    light: '#ff6b7a',
    dark: '#ff3838',
    contrastText: '#ffffff',
  },

  // Border colors
  border: 'hsl(220, 15%, 20%)',
  input: 'hsl(220, 15%, 20%)',

  // Other colors - using theme colors
  glow: '#b8d8f8', // Using secondary blue
  ring: '#e8f4ff', // Using primary blue

  // Radius
  radius: 8, // 0.5rem = 8px
};

// Create the MUI theme
const baseTheme = createTheme({
  palette: {
    mode: 'dark',
    background: {
      default: themeColors.background,
      paper: themeColors.card,
    },
    text: {
      primary: themeColors.foreground,
      secondary: themeColors.mutedForeground,
    },
    primary: themeColors.primary,
    secondary: themeColors.secondary,
    info: {
      main: themeColors.accent.main,
      contrastText: themeColors.accent.contrastText,
    },
    success: {
      main: '#10b981', // emerald-500
    },
    warning: {
      main: '#f59e0b', // amber-500
    },
    error: themeColors.destructive,
    divider: themeColors.border,
  },

  typography: {
    fontFamily: '"Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
    h1: {
      fontSize: '2.25rem',
      fontWeight: 700,
      lineHeight: 1.2,
    },
    h2: {
      fontSize: '1.875rem',
      fontWeight: 600,
      lineHeight: 1.3,
    },
    h3: {
      fontSize: '1.5rem',
      fontWeight: 600,
      lineHeight: 1.4,
    },
    h4: {
      fontSize: '1.25rem',
      fontWeight: 600,
      lineHeight: 1.4,
    },
    h5: {
      fontSize: '1.125rem',
      fontWeight: 600,
      lineHeight: 1.4,
    },
    h6: {
      fontSize: '1rem',
      fontWeight: 600,
      lineHeight: 1.4,
    },
    body1: {
      fontSize: '1rem',
      lineHeight: 1.6,
    },
    body2: {
      fontSize: '0.875rem',
      lineHeight: 1.6,
    },
    subtitle1: {
      fontSize: '1rem',
      fontWeight: 500,
      lineHeight: 1.5,
    },
    subtitle2: {
      fontSize: '0.875rem',
      fontWeight: 500,
      lineHeight: 1.5,
    },
    caption: {
      fontSize: '0.75rem',
      lineHeight: 1.4,
    },
    overline: {
      fontSize: '0.75rem',
      fontWeight: 500,
      letterSpacing: '0.08em',
      textTransform: 'uppercase',
    },
    button: {
      fontSize: '0.875rem',
      fontWeight: 500,
      textTransform: 'none',
    },
  },

  shape: {
    borderRadius: themeColors.radius,
  },

  shadows: [
    'none',
    `0px 2px 1px -1px rgba(0,0,0,0.2),0px 1px 1px 0px rgba(0,0,0,0.14),0px 1px 3px 0px rgba(0,0,0,0.12)`,
    `0px 3px 1px -2px rgba(0,0,0,0.2),0px 2px 2px 0px rgba(0,0,0,0.14),0px 1px 5px 0px rgba(0,0,0,0.12)`,
    `0px 3px 3px -2px rgba(0,0,0,0.2),0px 3px 4px 0px rgba(0,0,0,0.14),0px 1px 8px 0px rgba(0,0,0,0.12)`,
    `0px 2px 4px -1px rgba(0,0,0,0.2),0px 4px 5px 0px rgba(0,0,0,0.14),0px 1px 10px 0px rgba(0,0,0,0.12)`,
    `0px 3px 5px -1px rgba(0,0,0,0.2),0px 5px 8px 0px rgba(0,0,0,0.14),0px 1px 14px 0px rgba(0,0,0,0.12)`,
    `0px 3px 5px -1px rgba(0,0,0,0.2),0px 6px 10px 0px rgba(0,0,0,0.14),0px 1px 18px 0px rgba(0,0,0,0.12)`,
    `0px 4px 5px -2px rgba(0,0,0,0.2),0px 7px 10px 1px rgba(0,0,0,0.14),0px 2px 16px 1px rgba(0,0,0,0.12)`,
    `0px 5px 5px -3px rgba(0,0,0,0.2),0px 8px 10px 1px rgba(0,0,0,0.14),0px 3px 14px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    // Add custom glow shadow
    `0 0 20px ${themeColors.glow}33, 0 0 40px ${themeColors.glow}1a`,
    // Custom card shadow
    `0px 4px 6px -1px rgba(0,0,0,0.3),0px 2px 4px -1px rgba(0,0,0,0.2)`,
    // Additional shadows to match MUI's 24 array length
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
    `0px 5px 6px -3px rgba(0,0,0,0.2),0px 9px 12px 1px rgba(0,0,0,0.14),0px 3px 16px 2px rgba(0,0,0,0.12)`,
  ],

  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          textTransform: 'none',
          fontWeight: 500,
          borderRadius: '0.375rem',
          transition: 'all 0.2s ease-in-out',
          gap: '0.5rem',
        },
        contained: {
          background: `linear-gradient(135deg, ${themeColors.primary.main}, ${themeColors.secondary.main})`,
          color: themeColors.primary.contrastText,
          '&:hover': {
            background: `linear-gradient(135deg, ${themeColors.primary.dark}, ${themeColors.secondary.dark})`,
            transform: 'translateY(-1px)',
            boxShadow: `0 4px 8px -2px ${themeColors.primary.main}40`,
          },
        },
        outlined: {
          borderColor: themeColors.border,
          '&:hover': {
            backgroundColor: 'rgba(255, 255, 255, 0.05)',
            borderColor: themeColors.primary.main,
          },
        },
        text: {
          '&:hover': {
            backgroundColor: 'rgba(255, 255, 255, 0.05)',
          },
        },
      },
    },

    MuiCard: {
      styleOverrides: {
        root: {
          backgroundColor: themeColors.card,
          border: `1px solid ${themeColors.border}`,
          borderRadius: '0.5rem',
          transition: 'all 0.2s ease-in-out',
          '&:hover': {
            boxShadow: `0 4px 12px ${themeColors.primary.main}20`,
            borderColor: themeColors.primary.main,
            transform: 'translateY(-2px)',
          },
        },
      },
    },

    MuiCardContent: {
      styleOverrides: {
        root: {
          padding: '1.5rem',
        },
      },
    },

    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          backgroundColor: themeColors.popover,
          color: themeColors.popoverForeground,
          border: `1px solid ${themeColors.border}`,
          fontSize: '0.875rem',
          padding: '0.375rem 0.75rem',
          borderRadius: '0.375rem',
          maxWidth: '300px',
          fontWeight: 400,
          boxShadow: 'var(--shadow)',
        },
        arrow: {
          color: themeColors.popover,
          '&::before': {
            border: `1px solid ${themeColors.border}`,
          },
        },
      },
    },

    MuiLink: {
      styleOverrides: {
        root: {
          color: themeColors.primary.main,
          textDecoration: 'none',
          '&:hover': {
            color: themeColors.primary.light,
            textDecoration: 'underline',
          },
        },
      },
    },

    MuiTextField: {
      styleOverrides: {
        root: {
          '& .MuiOutlinedInput-root': {
            backgroundColor: 'rgba(255, 255, 255, 0.02)',
            '&:hover fieldset': {
              borderColor: themeColors.primary.main,
            },
            '&.Mui-focused fieldset': {
              borderColor: themeColors.primary.main,
              borderWidth: '2px',
            },
          },
        },
      },
    },

    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
        },
        elevation1: {
          boxShadow: '0px 2px 1px -1px rgba(0,0,0,0.2),0px 1px 1px 0px rgba(0,0,0,0.14),0px 1px 3px 0px rgba(0,0,0,0.12)',
        },
        elevation2: {
          boxShadow: '0px 3px 1px -2px rgba(0,0,0,0.2),0px 2px 2px 0px rgba(0,0,0,0.14),0px 1px 5px 0px rgba(0,0,0,0.12)',
        },
        elevation3: {
          boxShadow: '0px 3px 3px -2px rgba(0,0,0,0.2),0px 3px 4px 0px rgba(0,0,0,0.14),0px 1px 8px 0px rgba(0,0,0,0.12)',
        },
      },
    },
  },

});

// Export the theme
export const theme = {
  ...baseTheme,
  custom: {
    glow: `0 0 20px ${themeColors.glow}40, 0 0 40px ${themeColors.glow}20`,
    textGlow: `0 0 20px ${themeColors.glow}60, 0 0 40px ${themeColors.glow}30`,
    gradientBorder: `linear-gradient(${themeColors.background}, ${themeColors.background}) padding-box,
                     linear-gradient(135deg, ${themeColors.primary.main}, ${themeColors.secondary.main}) border-box`,
  },
};

// Export theme colors for use in components
export const colors = themeColors;

// Component utility styles
export const componentStyles = {
  // Section headers
  sectionHeader: "text-[10px] uppercase tracking-widest font-bold",
  sectionHeaderCyan: "text-cyan-400",
  sectionHeaderBlue: "text-blue-400",
  sectionHeaderMuted: "text-muted-foreground",

  // Status indicator
  statusIndicator: {
    container: "flex items-center gap-2 text-[10px] font-bold uppercase tracking-widest text-cyan-500/80",
    pulse: "flex h-2 w-2 rounded-full bg-cyan-500 animate-pulse",
  },

  // URL display
  urlDisplay: "text-xs sm:text-sm text-cyan-400 font-mono break-all block bg-black/40 p-3 rounded-lg border border-cyan-500/20",

  // Image with overlay
  imageWithOverlay: {
    container: "relative group overflow-hidden rounded-lg border border-white/10 bg-black/20",
    image: "opacity-80 group-hover:opacity-100 transition-opacity duration-300",
    overlay: "absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent flex items-end p-3",
    overlayText: "text-[11px] text-white/70 flex items-center gap-2",
  },

  // Info box
  infoBox: {
    container: "flex gap-3 bg-white/5 p-3 rounded-lg",
    icon: "text-cyan-400 shrink-0",
    text: "text-xs text-muted-foreground leading-relaxed",
  },

  // Link box
  linkBox: {
    container: "space-y-3 p-3 sm:p-4 bg-secondary/20 rounded-lg border border-white/5",
    url: "text-xs sm:text-sm text-cyan-400 font-mono break-all block bg-black/20 p-2 rounded",
    note: "text-xs text-muted-foreground italic",
  },
};

// Export types and hook
export * from "./types";
export { useTheme } from "./use-theme";

export default theme;