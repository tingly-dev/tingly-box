import { createTheme } from '@mui/material/styles';
import type { ThemeMode } from './types';
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

const createAppTheme = (mode: ThemeMode) => {
  const { palette, components } = THEME_REGISTRY[mode];
  const textColors = palette.text as { primary: string; secondary: string; disabled: string };

  return createTheme({
    palette: palette as any,
    typography: baseTypography(textColors.primary, textColors.secondary, textColors.disabled),
    shape: baseShape,
    components: {
      ...baseComponents,
      ...components,
    },
  });
};

export default createAppTheme;
export type { ThemeMode } from './types';
