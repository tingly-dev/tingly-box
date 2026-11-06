// A one-time callout pointing at a control, to teach where something is
// (ux-principles #8). It sits beside its anchor with an arrow back at it and
// closes on its button; callers pair it with their own one-time-tip storage
// so it never returns. This is a page primitive shared by Tingly Box and the
// embedded Projects pages, so every string comes in as a prop — the package
// carries no i18n of its own.
import { Button, Paper, Popper, Stack, Typography } from '@mui/material';
import type { FC } from 'react';
import { Z_INDEX } from '../theme/zIndex';

interface CoachMarkProps {
    open: boolean;
    anchorEl: HTMLElement | null;
    title: string;
    text: string;
    /** The dismiss button's label ("Got it"). */
    gotItLabel: string;
    onDismiss: () => void;
    /**
     * 'top': the callout's top lines up with the anchor (a control near the
     * top of the screen). 'bottom': its bottom does (a control in a footer).
     */
    align?: 'top' | 'bottom';
}

export const CoachMark: FC<CoachMarkProps> = ({ open, anchorEl, title, text, gotItLabel, onDismiss, align = 'top' }) => {
    const arrowSide = align === 'top' ? { top: 14 } : { bottom: 14 };

    return (
        <Popper
            open={open && !!anchorEl}
            anchorEl={anchorEl}
            placement={align === 'top' ? 'right-start' : 'right-end'}
            modifiers={[{ name: 'offset', options: { offset: [align === 'top' ? -8 : 8, 14] } }]}
            sx={{ zIndex: Z_INDEX.drawer + 3 }}
        >
            <Paper
                elevation={6}
                role="dialog"
                aria-label={title}
                sx={{
                    position: 'relative',
                    width: 260,
                    p: 1.75,
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: 'primary.main',
                    // Arrow pointing back at the anchor.
                    '&::before': {
                        content: '""',
                        position: 'absolute',
                        left: -7,
                        ...arrowSide,
                        width: 12,
                        height: 12,
                        bgcolor: 'background.paper',
                        borderLeft: '1px solid',
                        borderBottom: '1px solid',
                        borderColor: 'primary.main',
                        transform: 'rotate(45deg)',
                    },
                }}
            >
                <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.5 }}>
                    {title}
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                    {text}
                </Typography>
                <Stack direction="row" sx={{ justifyContent: 'flex-end' }}>
                    <Button size="small" variant="contained" disableElevation onClick={onDismiss}>
                        {gotItLabel}
                    </Button>
                </Stack>
            </Paper>
        </Popper>
    );
};
