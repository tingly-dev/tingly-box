import type { Components, Theme } from '@mui/material/styles';

type ButtonVariants = NonNullable<Components<Theme>['MuiButton']>['variants'];

/**
 * The brand gradient, scoped to the primary contained button.
 *
 * It has to be a `variants` entry rather than a `styleOverrides.contained`
 * rule: an override on the bare `contained` slot cannot see the colour, so it
 * paints `color="error"` buttons too and every destructive confirmation in the
 * app comes out looking like a primary action. Each theme brings its own four
 * stops; the reason they are scoped this way is written down once, here.
 *
 * `disabled: false` in the matcher matters just as much as `color: 'primary'`
 * does: without it, this variant also matches a disabled button (MUI's
 * `variants` mechanism only matches on `variant`/`color`/etc, not `disabled`)
 * and its gradient background paints *over* MUI's own `.Mui-disabled` style
 * (which would otherwise fall back to `action.disabledBackground`/
 * `action.disabled`). The result was disabled primary buttons rendering at
 * full brand-color saturation — indistinguishable from an enabled button,
 * and with the (correctly muted) disabled label text nearly invisible
 * against that bright fill.
 */
export const primaryGradientButton = (
    from: string,
    to: string,
    hoverFrom: string,
    hoverTo: string,
): ButtonVariants => [
    {
        props: { variant: 'contained', color: 'primary', disabled: false },
        style: {
            background: `linear-gradient(135deg, ${from} 0%, ${to} 100%)`,
            '&:hover': {
                background: `linear-gradient(135deg, ${hoverFrom} 0%, ${hoverTo} 100%)`,
            },
        },
    },
];
