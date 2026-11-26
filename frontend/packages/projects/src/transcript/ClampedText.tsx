import {Box, ButtonBase, Typography} from '@mui/material';
import type {Theme} from '@mui/material/styles';
import {ExpandMore} from '@tingly/ui/icons';
import {fontSizes} from '@tingly/ui/theme/fonts';
import type {ReactNode} from 'react';
import {useLayoutEffect, useId, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';

interface Props {
    /** Lines shown while collapsed, on the phone. */
    lines: number;
    children: ReactNode;
    /** Inside the user's bubble: the row spans its bottom edge and reuses its corners. */
    bubble?: boolean;
}

// ClampedText is the phone's long-text guard: a measured line clamp whose
// 展开/收起 row is the block's last row, inside the bubble/card, with a bottom
// mask fading the last line into it. The row has its own 44px, so it never
// covers the last full line; the full text stays in the DOM (copy/selection and
// assistive tech see all of it). Short text gets no row.
const ClampedText = ({lines, children, bubble = false}: Props) => {
    const {t} = useTranslation();
    const id = useId();
    const body = useRef<HTMLDivElement>(null);
    const [overflows, setOverflows] = useState(false);
    const [open, setOpen] = useState(false);

    useLayoutEffect(() => {
        const el = body.current;
        if (!el || open) return;
        const measure = () => setOverflows(el.scrollHeight > el.clientHeight + 1);
        measure();
        if (typeof ResizeObserver === 'undefined') return;
        const observer = new ResizeObserver(measure);
        observer.observe(el);
        return () => observer.disconnect();
    }, [open]);

    // The clamp is the measuring state: apply it whenever collapsed, then the
    // effect finds whether it actually hides anything. The mask (more below)
    // appears only then; short text is not clipped and gets no row.
    const clamped = !open;
    const fading = overflows && !open;
    return (
        <Box>
            <Box
                ref={body}
                id={id}
                sx={{
                    ...(clamped && {
                        display: '-webkit-box',
                        WebkitBoxOrient: 'vertical',
                        WebkitLineClamp: lines,
                        overflow: 'hidden',
                    }),
                    ...(fading && {
                        WebkitMaskImage: (theme: Theme) => `linear-gradient(to bottom, ${theme.palette.common.black} calc(100% - 30px), transparent)`,
                        maskImage: (theme: Theme) => `linear-gradient(to bottom, ${theme.palette.common.black} calc(100% - 30px), transparent)`,
                    }),
                }}
            >
                {children}
            </Box>
            {(overflows || open) && (
                <ButtonBase
                    onClick={() => setOpen((value) => !value)}
                    aria-expanded={open}
                    aria-controls={id}
                    sx={{
                        width: '100%',
                        minHeight: 44,
                        justifyContent: 'flex-start',
                        gap: 0.75,
                        mt: 1,
                        px: bubble ? 2 : 0.5,
                        color: 'primary.main',
                        textAlign: 'left',
                        ...(bubble && {
                            mx: -2,
                            mb: -1.25,
                            mt: 1.5,
                            pt: 1,
                            width: 'calc(100% + 32px)',
                            borderTop: 1,
                            borderColor: 'divider',
                        }),
                    }}
                >
                    <Typography component="span" sx={{fontSize: fontSizes.sm, fontWeight: 600}}>
                        {open
                            ? t('projects.transcript.collapseText', {defaultValue: 'Show less'})
                            : t('projects.transcript.expandText', {defaultValue: 'Show full text'})}
                    </Typography>
                    <ExpandMore sx={{fontSize: 16, transform: open ? 'rotate(180deg)' : 'none'}}/>
                </ButtonBase>
            )}
        </Box>
    );
};

export default ClampedText;
