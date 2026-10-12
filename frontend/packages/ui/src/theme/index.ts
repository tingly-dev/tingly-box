import { createTheme } from '@mui/material/styles';
import { enUS, ruRU, zhCN } from '@mui/material/locale';
import type { ResolvedThemeMode } from './types';
import { baseTypography, baseShape, baseComponents } from './base';
import { lightPalette } from './palettes/light';
import { darkPalette } from './palettes/dark';
import { sunlitPalette } from './palettes/sunlit';
import { claudePalette } from './palettes/claude';
import { dsPalette } from './palettes/ds';
import { lightComponents } from './components/light';
import { darkComponents } from './components/dark';
import { sunlitComponents } from './components/sunlit';
import { claudeComponents } from './components/claude';
import { dsComponents } from './components/ds';

const THEME_REGISTRY = {
  light: { palette: lightPalette, components: lightComponents },
  dark: { palette: darkPalette, components: darkComponents },
  sunlit: { palette: sunlitPalette, components: sunlitComponents },
  claude: { palette: claudePalette, components: claudeComponents },
  ds: { palette: dsPalette, components: dsComponents },
} as const;

// Keyed by the app's language codes; anything else falls back to English.
const MUI_LOCALES: Record<string, typeof enUS> = { en: enUS, zh: zhCN, ru: ruRU };

const createAppTheme = (mode: ResolvedThemeMode, language = 'en') => {
  const { palette, components } = THEME_REGISTRY[mode];
  const textColors = palette.text as { primary: string; secondary: string; disabled: string };

  return createTheme(
    {
      palette: palette as any,
      typography: baseTypography(textColors.primary, textColors.secondary, textColors.disabled),
      shape: baseShape,
      components: {
        ...baseComponents,
        ...components,
      },
    },
    // MUI locale — localizes built-in text like TablePagination labels,
    // while our own UI strings go through i18next (see DashboardPage).
    MUI_LOCALES[language] ?? enUS,
  );
};

export default createAppTheme;
export type { ResolvedThemeMode, ThemeMode } from './types';
