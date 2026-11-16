import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

/** Composer menus open above their trigger to leave room at the bottom of the page. */
export const composerPickerMenuProps = {
    anchorOrigin: { vertical: 'top', horizontal: 'right' },
    transformOrigin: { vertical: 'bottom', horizontal: 'right' },
} as const;

/**
 * The look of a picker that sits in the message box (a model, a profile, a
 * permission mode): quiet text with an icon, no border of its own, so a row
 * of them reads as one strip and the text stays the subject. Hosts spread it
 * into a standard, underline-free Select's `sx`.
 */
export const composerPickerSx = {
    px: 0.75,
    borderRadius: radius.row,
    maxWidth: '100%',
    minWidth: 0,
    color: 'text.secondary',
    fontSize: fontSizes.sm,
    '&:hover': { bgcolor: 'action.hover', color: 'text.primary' },
    '& .MuiSelect-select': {
        py: { xs: 0.75, md: 0.5 }, pr: '18px !important', minHeight: 'initial !important',
        display: 'flex', alignItems: 'center', bgcolor: 'transparent !important',
    },
    '& .MuiSelect-icon': { fontSize: 16, right: 0, color: 'inherit' },
    '& svg:not(.MuiSelect-icon)': { fontSize: 14 },
} as const;

/** The picker's label: the same size as the strip, inheriting its color. */
export const composerPickerLabelSx = { fontSize: fontSizes.sm, lineHeight: 1.4, color: 'inherit', minWidth: 0 } as const;
