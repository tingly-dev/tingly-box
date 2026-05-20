import type { PaletteOptions, ThemeOptions } from '@mui/material/styles';

export type ResolvedThemeMode = 'light' | 'dark' | 'sunlit' | 'claude';
export type ThemeMode = ResolvedThemeMode | 'system';

export interface DashboardTokenColors {
  main: string;
  gradient: string;
}

export interface DashboardColors {
  token: {
    input: DashboardTokenColors;
    output: DashboardTokenColors;
    cache: DashboardTokenColors;
  };
  chart: {
    grid: string;
    axis: string;
    tooltipBg: string;
    tooltipBorder: string;
  };
  statCard: {
    boxShadow: string;
    emptyIconBg: string;
  };
}

export interface ThemePalette extends PaletteOptions {
  dashboard: DashboardColors;
  isSunlit: boolean;
}

export interface ThemeConfig {
  palette: ThemePalette;
  components: ThemeOptions['components'];
}
