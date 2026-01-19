import { Theme } from "@mui/material/styles";

// Declare theme augmentation module
declare module "@mui/material/styles" {
  interface Theme {
    custom: {
      glow: string;
      textGlow: string;
      gradientBorder: string;
    };
  }
}

// Create a type that extends the default MUI theme
export interface AppTheme extends Theme {
  custom: {
    glow: string;
    textGlow: string;
    gradientBorder: string;
  };
}

// Export theme configuration types
export interface ThemeColors {
  background: string;
  foreground: string;
  card: string;
  cardForeground: string;
  popover: string;
  popoverForeground: string;
  primary: {
    main: string;
    light: string;
    dark: string;
    contrastText: string;
  };
  secondary: {
    main: string;
    light: string;
    dark: string;
    contrastText: string;
  };
  muted: string;
  mutedForeground: string;
  accent: {
    main: string;
    contrastText: string;
  };
  destructive: {
    main: string;
    light: string;
    dark: string;
    contrastText: string;
  };
  border: string;
  input: string;
  glow: string;
  ring: string;
  radius: number;
}

// Export component prop types
export interface ThemedComponentProps {
  className?: string;
  sx?: object;
}

// Export theme hooks type
export interface UseThemeResult {
  theme: AppTheme;
  colors: ThemeColors;
}