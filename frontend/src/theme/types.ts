import type { PaletteOptions, ThemeOptions } from '@mui/material/styles';

export type ThemeMode = 'light' | 'dark' | 'sunlit' | 'claude';

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
