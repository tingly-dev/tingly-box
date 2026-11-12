import { createTheme, type ThemeOptions } from '@mui/material/styles';
import { enUS, ruRU, zhCN } from '@mui/material/locale';
import type { ResolvedThemeMode } from './types';
import type { ThemeTokens } from './tokens';
import { baseTypography, baseShape, baseComponents } from './base';
import { lightPalette, lightTokens } from './palettes/light';
import { darkPalette, darkTokens } from './palettes/dark';
import { claudePalette, claudeTokens } from './palettes/claude';
import { dsPalette, dsTokens } from './palettes/ds';
import { createComponents } from './components/factory';
import { dsExtraComponents } from './components/ds';

// A theme is a palette plus a token table; the component overrides come from
// one factory. `extras` is for the rare theme that styles more than colour.
const THEME_REGISTRY: Record<ResolvedThemeMode, {
  palette: ThemeOptions['palette'];
  tokens: ThemeTokens;
  extras?: ThemeOptions['components'];
}> = {
  light: { palette: lightPalette, tokens: lightTokens },
  dark: { palette: darkPalette, tokens: darkTokens },
  claude: { palette: claudePalette, tokens: claudeTokens },
  ds: { palette: dsPalette, tokens: dsTokens, extras: dsExtraComponents },
};

// Keyed by the app's language codes; anything else falls back to English.
const MUI_LOCALES: Record<string, typeof enUS> = { en: enUS, zh: zhCN, ru: ruRU };

const createAppTheme = (mode: ResolvedThemeMode, language = 'en') => {
  const { palette, tokens, extras } = THEME_REGISTRY[mode];
  const textColors = palette!.text as { primary: string; secondary: string; disabled: string };

  // createTheme deep-merges its extra arguments in order, so a theme's
  // overrides add to the shared ones per style key instead of replacing a
  // component's whole block.
  return createTheme(
    {
      palette,
      typography: baseTypography(textColors.primary, textColors.secondary, textColors.disabled),
      shape: baseShape,
      components: baseComponents,
    },
    { components: createComponents(tokens) },
    { components: extras ?? {} },
    // MUI locale — localizes built-in text like TablePagination labels,
    // while our own UI strings go through i18next (see DashboardPage).
    MUI_LOCALES[language] ?? enUS,
  );
};

export default createAppTheme;
export type { ResolvedThemeMode, ThemeMode } from './types';
export type { ThemeTokens } from './tokens';
