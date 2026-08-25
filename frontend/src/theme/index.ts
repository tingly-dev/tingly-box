import { createTheme } from '@mui/material/styles';
import { arEG, enUS, faIR, ruRU, zhCN } from '@mui/material/locale';
import type { ResolvedThemeMode } from './types';
import { DEFAULT_LANGUAGE, directionOf, type AppLanguage } from '@/i18n';
import { baseTypography, baseShape, baseComponents } from './base';
import { lightPalette } from './palettes/light';
import { darkPalette } from './palettes/dark';
import { sunlitPalette } from './palettes/sunlit';
import { claudePalette } from './palettes/claude';
import { lightComponents } from './components/light';
import { darkComponents } from './components/dark';
import { sunlitComponents } from './components/sunlit';
import { claudeComponents } from './components/claude';

const THEME_REGISTRY = {
  light: { palette: lightPalette, components: lightComponents },
  dark: { palette: darkPalette, components: darkComponents },
  sunlit: { palette: sunlitPalette, components: sunlitComponents },
  claude: { palette: claudePalette, components: claudeComponents },
} as const;

const MUI_LOCALES = { en: enUS, zh: zhCN, ru: ruRU, fa: faIR, ar: arEG } as const;

const createAppTheme = (mode: ResolvedThemeMode, language: AppLanguage = DEFAULT_LANGUAGE) => {
  const { palette, components } = THEME_REGISTRY[mode];
  const textColors = palette.text as { primary: string; secondary: string; disabled: string };

  return createTheme(
    {
      // Drives MUI's own RTL handling (Drawer anchors, Menu/Popper placement,
      // Slider direction, …) via RtlProvider. The app's own physical CSS is
      // mirrored separately by the emotion RTL cache in App.tsx.
      direction: directionOf(language),
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
