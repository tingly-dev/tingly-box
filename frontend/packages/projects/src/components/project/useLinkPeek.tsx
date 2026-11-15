import { useCallback, useEffect, useRef, useState, type PointerEvent, type ReactNode } from 'react';
import { Box, Drawer, IconButton, Paper, Popover, Popper, useMediaQuery } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { mobile, PHONE_QUERY, safeAreaBottom } from '@tingly/ui/theme/mobile';
import { api, unwrap, type ThreadLink } from '../../client';
import { describeOutcome, useLinkRefresh } from '../../hooks/useLinkRefresh';
import { Info, Refresh, Spinner } from '../icons';
import { LinkDetails } from './LinkDetails';
import { spin, visuallyHidden } from './linkView';

/** How long the pointer rests on a row before its details float up, and how long it may leave (to cross to the popover) before they go. */
const HOVER_OPEN_MS = 350;
const HOVER_CLOSE_MS = 200;

/** The class the row's hover rule reveals: refresh and details are hidden until the row is looked at, never out of the tab order, and always there where there is no hover. */
export const peekRowSx = {
    '& .link-act': { opacity: 0, transition: 'opacity 120ms' },
    '&:hover .link-act, &:focus-within .link-act, & .link-act[data-keep]': { opacity: 1 },
    '@media (hover: none)': { '& .link-act': { opacity: 1 } },
} as const;

/**
 * What every row of an issue or PR shares: a refresh and a details button, the
 * details floating up when the pointer rests on the row (a preview that takes
 * neither focus nor the pointer) or staying put when the button is pressed (a
 * popover, a sheet on a phone), and the refresh's own feedback. A row spreads
 * `row` on its container (it is the anchor), puts `actions` at its end and
 * `overlay` anywhere inside it. `above` opens upward, for a row at the bottom
 * of the screen.
 */
export function useLinkPeek(link: ThreadLink, threadId: string | undefined, above = false): { row: { ref: (el: HTMLElement | null) => void; onPointerEnter: (e: PointerEvent<HTMLElement>) => void; onPointerLeave: () => void }; actions: ReactNode; overlay: ReactNode } {
    const { t } = useTranslation('projects');
    const phone = useMediaQuery(PHONE_QUERY);
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const detailsRef = useRef<HTMLButtonElement>(null);
    const [mode, setMode] = useState<'hover' | 'pinned' | null>(null);
    const timer = useRef<number | undefined>(undefined);
    useEffect(() => () => window.clearTimeout(timer.current), []);
    const call = useCallback(
        () => unwrap(api.POST('/api/v1/threads/{id}/links/refresh', { params: { path: { id: threadId ?? '' } }, body: { repo: link.repo, kind: link.kind as 'issue' | 'pr', number: link.number } })),
        [threadId, link.repo, link.kind, link.number],
    );
    const { busy, outcome, run } = useLinkRefresh(call);
    const said = describeOutcome(outcome, t);

    const later = (fn: () => void, ms: number) => {
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(fn, ms);
    };
    const keep = () => window.clearTimeout(timer.current);
    // Not every browser focuses a pressed button, so the focus is put back by hand.
    const closePinned = () => {
        setMode(null);
        detailsRef.current?.focus();
    };
    const leave = () => later(() => setMode((m) => (m === 'hover' ? null : m)), HOVER_CLOSE_MS);
    const enter = (e: PointerEvent<HTMLElement>) => {
        // Touch has no hover: its way in is the details button.
        if (e.pointerType !== 'mouse') return;
        keep();
        later(() => setMode((m) => m ?? 'hover'), HOVER_OPEN_MS);
    };
    useEffect(() => {
        if (mode !== 'hover') return;
        const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setMode(null);
        document.addEventListener('keydown', onKey);
        return () => document.removeEventListener('keydown', onKey);
    }, [mode]);

    const details = <LinkDetails link={link} busy={busy} outcome={said} onRefresh={threadId ? () => void run() : undefined} fill={phone} />;
    const actionSx = { p: 0.25, color: 'text.secondary', '@media (pointer: coarse)': { minWidth: mobile.tapMin, minHeight: mobile.tapMin } } as const;
    const actions = (
        <Box className="link-act" data-keep={busy || mode === 'pinned' ? '' : undefined} sx={{ display: 'flex', flexShrink: 0, alignItems: 'center' }}>
            {threadId && (
                <IconButton size="small" aria-label={busy ? t('links.refreshing') : t('links.refresh')} title={t('links.refresh')} aria-busy={busy} disabled={busy} onClick={() => void run()} sx={actionSx}>
                    {busy ? <Spinner sx={{ fontSize: 16, animation: `${spin} 1s linear infinite` }} /> : <Refresh sx={{ fontSize: 16 }} />}
                </IconButton>
            )}
            <IconButton ref={detailsRef} size="small" aria-label={t('links.detailsOf', { ref: `${link.repo}#${link.number}` })} title={t('links.details')} aria-haspopup="dialog" aria-expanded={mode === 'pinned'}
                onClick={() => { keep(); setMode((m) => (m === 'pinned' ? null : 'pinned')); }} sx={actionSx}>
                <Info sx={{ fontSize: 16 }} />
            </IconButton>
        </Box>
    );
    const overlay = (
        <>
            <Box role="status" sx={visuallyHidden}>{said}</Box>
            {mode === 'hover' && !phone && (
                // A preview floats over the page without hiding it from a screen reader, taking focus or the pointer; pressing the button makes it a real popover.
                <Popper open anchorEl={anchor} placement={above ? 'top-start' : 'bottom-start'} sx={{ zIndex: (theme) => theme.zIndex.modal }}>
                    <Paper role="dialog" aria-label={link.title} elevation={8} onPointerEnter={keep} onPointerLeave={leave} sx={{ maxWidth: '92vw' }}>{details}</Paper>
                </Popper>
            )}
            {mode === 'pinned' && (phone
                ? <Drawer anchor="bottom" open onClose={closePinned} slotProps={{ paper: { role: 'dialog', 'aria-label': link.title, sx: { borderTopLeftRadius: (theme) => theme.spacing(mobile.sheetRadius), borderTopRightRadius: (theme) => theme.spacing(mobile.sheetRadius), pb: safeAreaBottom } } }}>{details}</Drawer>
                : <Popover open anchorEl={anchor} onClose={closePinned}
                    anchorOrigin={{ vertical: above ? 'top' : 'bottom', horizontal: 'left' }} transformOrigin={{ vertical: above ? 'bottom' : 'top', horizontal: 'left' }}
                    slotProps={{ paper: { role: 'dialog', 'aria-label': link.title, sx: { maxWidth: '92vw' } } }}>
                    {details}
                </Popover>)}
        </>
    );
    return { row: { ref: setAnchor, onPointerEnter: enter, onPointerLeave: leave }, actions, overlay };
}
