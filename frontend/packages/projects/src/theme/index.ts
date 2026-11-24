// The theme is Tingly Box's, from @tingly/ui (a workspace package). Only the
// language mapping is tingly-project's.
import createTinglyTheme, { type ResolvedThemeMode } from '@tingly/ui/theme';

export type { ResolvedThemeMode, ThemeMode } from '@tingly/ui/theme';

export function createAppTheme(mode: ResolvedThemeMode, language: string | undefined) {
    return createTinglyTheme(mode, language?.startsWith('zh') ? 'zh' : 'en');
}
