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
 */
export const primaryGradientButton = (
    from: string,
    to: string,
    hoverFrom: string,
    hoverTo: string,
): ButtonVariants => [
    {
        props: { variant: 'contained', color: 'primary' },
        style: {
            background: `linear-gradient(135deg, ${from} 0%, ${to} 100%)`,
            '&:hover': {
                background: `linear-gradient(135deg, ${hoverFrom} 0%, ${hoverTo} 100%)`,
            },
        },
    },
];
