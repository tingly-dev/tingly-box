// Theme modes and their icons come from Tingly Box's theme switcher, so both
// apps offer the same set with the same marks. Labels stay ours (i18n
// `theme.<mode>`), so the resolver handed to getThemeOptions is unused.
import type { SxProps, Theme } from '@mui/material';
import { getThemeOptions } from '@tingly/ui/theme/options';
import type { ResolvedThemeMode, ThemeMode } from '@tingly/ui/theme';

export { THEME_MODE_VALUES, isThemeMode } from '@tingly/ui/theme/options';

export const resolveThemeMode = (mode: ThemeMode, prefersDark: boolean): ResolvedThemeMode =>
    mode === 'system' ? (prefersDark ? 'dark' : 'light') : mode;

const ICONS = Object.fromEntries(getThemeOptions((key) => key).map((o) => [o.value, o.renderIcon])) as Record<
    ThemeMode,
    ReturnType<typeof getThemeOptions>[number]['renderIcon']
>;

export function ThemeModeIcon({ mode, size = 18, sx }: { mode: ThemeMode; size?: number; sx?: SxProps<Theme> }) {
    const Icon = ICONS[mode];
    return <Icon size={size} sx={sx} />;
}
