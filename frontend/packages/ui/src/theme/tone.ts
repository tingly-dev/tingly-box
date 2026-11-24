import { alpha, type Theme } from '@mui/material/styles';

// A quiet colour tone for a badge, a tile or a ring: a tint of the palette
// colour with ink that stays legible on it. Same recipe as the soft selected
// look (selection.ts), so every tinted thing follows the theme and dark mode.
export type Tone = 'primary' | 'success' | 'warning' | 'error' | 'info' | 'neutral';

export const toneColors = (theme: Theme, tone: Tone) => {
    const dark = theme.palette.mode === 'dark';
    if (tone === 'neutral') {
        return {
            bg: theme.palette.action.hover,
            ink: theme.palette.text.secondary,
            line: theme.palette.divider,
            ring: 'transparent',
        };
    }
    const { main, light, dark: deep } = theme.palette[tone];
    const ink = dark ? light : deep;
    return { bg: alpha(main, dark ? 0.2 : 0.1), ink, line: ink, ring: alpha(main, 0.08) };
};
