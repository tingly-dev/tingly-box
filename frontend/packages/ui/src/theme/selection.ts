import { alpha, type Theme } from '@mui/material/styles';

// The selected look for choices inside a page — a filter chip, a slot, a
// segmented option: a tint of the primary colour with primary-coloured text.
// Solid primary is kept for navigation (nav.ts) and the primary button, so a
// page's own selections never compete with "where am I".
//
// The ink is `primary.dark` on light themes and `primary.light` on dark ones,
// so small text on the tint stays legible in every palette.

export const softSelectedColors = (theme: Theme) => {
    const dark = theme.palette.mode === 'dark';
    const { main, light, dark: deep } = theme.palette.primary;
    return {
        bg: alpha(main, dark ? 0.2 : 0.1),
        bgHover: alpha(main, dark ? 0.28 : 0.16),
        ink: dark ? light : deep,
    };
};

/** sx for a selected in-page option. */
export const softSelectedSx = (theme: Theme) => {
    const c = softSelectedColors(theme);
    return {
        backgroundColor: c.bg,
        color: c.ink,
        fontWeight: 600,
        '&:hover': { backgroundColor: c.bgHover },
    };
};

/** A row's delete / remove icon: neutral until hovered, red only then — the
 *  confirmation that follows carries the warning, not the resting row. */
export const quietDangerIconSx = {
    color: 'text.secondary',
    '&:hover': { color: 'error.main' },
} as const;
