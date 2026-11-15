import { useCallback, useEffect, useRef, useState, type PointerEvent } from 'react';
import { Box, Drawer, IconButton, Link, Paper, Popover, Popper, Stack, useMediaQuery } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { mobile, PHONE_QUERY, safeAreaBottom } from '@tingly/ui/theme/mobile';
import { api, unwrap, type Thread, type ThreadLink } from '../../client';
import { describeOutcome, useLinkRefresh } from '../../hooks/useLinkRefresh';
import { Info, Refresh, Spinner } from '../icons';
import { timeAgo } from '../../utils/format';
import { RailCard } from './infoRail';
import { LinkDetails } from './LinkDetails';
import { colorOf, linkParts, LinkIcon, spin, visuallyHidden } from './linkView';

/** How long the pointer rests on a row before its details float up, and how long it may leave (to cross to the popover) before they go. */
const HOVER_OPEN_MS = 350;
const HOVER_CLOSE_MS = 200;

/** One issue or PR: its number and title as a link to GitHub, and under it
 * where it stands and how old that reading is. At the row's end, a refresh and
 * a details button: on a fine pointer they appear when the row is hovered or
 * has focus (and stay in the tab order), on touch they are always there. The
 * details float up after the pointer rests on the row, or stay put when the
 * button is pressed (a sheet on a phone). */
export function LinkRow({ link, threadId, owner, onOpenOwner }: { link: ThreadLink; threadId?: string; owner?: string; onOpenOwner?: () => void }) {
    const { t, i18n } = useTranslation('projects');
    const phone = useMediaQuery(PHONE_QUERY);
    const parts = linkParts(link, t);
    const age = timeAgo(link.refreshed_at, i18n.language);
    const [row, setRow] = useState<HTMLDivElement | null>(null);
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

    const details = <LinkDetails link={link} owner={owner} busy={busy} outcome={said} onRefresh={threadId ? () => void run() : undefined} fill={phone} />;
    const actionSx = { p: 0.25, color: 'text.secondary', '@media (pointer: coarse)': { minWidth: mobile.tapMin, minHeight: mobile.tapMin } } as const;
    return (
        <Stack ref={setRow} direction="row" spacing={0.75} onPointerEnter={enter} onPointerLeave={leave}
            sx={{
                alignItems: 'flex-start', minWidth: 0, py: 0.625, borderTop: 1, borderColor: 'divider',
                // Hidden until the row is looked at, but never out of the tab order; always there where there is no hover.
                '& .link-act': { opacity: 0, transition: 'opacity 120ms' },
                '&:hover .link-act, &:focus-within .link-act, & .link-act[data-keep]': { opacity: 1 },
                '@media (hover: none)': { '& .link-act': { opacity: 1 } },
            }}>
            <LinkIcon link={link} />
            <Box sx={{ minWidth: 0, flex: 1 }}>
                <Link href={link.url.startsWith('https://') ? link.url : undefined} target="_blank" rel="noopener noreferrer" underline="hover" color="text.primary" title={`${link.repo}#${link.number}`}
                    sx={{ display: 'block', fontSize: fontSizes.sm, lineHeight: 1.4, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    #{link.number} {link.title}
                </Link>
                <Box sx={{ display: 'flex', flexWrap: 'wrap', columnGap: 0.75, fontSize: fontSizes.sm, color: 'text.secondary', lineHeight: 1.4 }}>
                    {owner && (onOpenOwner
                        ? <Link component="button" type="button" underline="hover" color="text.secondary" onClick={onOpenOwner} title={t('links.openThread', { title: owner })}
                            sx={{ minWidth: 0, maxWidth: '100%', fontSize: 'inherit', textAlign: 'left', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{owner}</Link>
                        : <Box component="span" sx={{ minWidth: 0, maxWidth: '100%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{owner}</Box>)}
                    {parts.map((part) => (
                        <Box key={part.text} component="span" sx={{ color: (theme) => colorOf(theme, part.tone) }}>{part.text}</Box>
                    ))}
                    {link.last_error
                        ? <Box component="span" title={link.last_error} sx={{ color: (theme) => colorOf(theme, 'warning') }}>{t('links.stale', { age })}</Box>
                        : link.state === 'open' && age && <Box component="span" title={t('links.readTitle')}>{age}</Box>}
                </Box>
            </Box>
            <Box className="link-act" data-keep={busy || mode === 'pinned' ? '' : undefined} sx={{ display: 'flex', flexShrink: 0, alignItems: 'center', mt: '-2px' }}>
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
            <Box role="status" sx={visuallyHidden}>{said}</Box>
            {mode === 'hover' && !phone && (
                // A preview floats over the page without hiding it from a screen reader, taking focus or the pointer; pressing the button makes it a real popover.
                <Popper open anchorEl={row} placement="bottom-start" sx={{ zIndex: (theme) => theme.zIndex.modal }}>
                    <Paper role="dialog" aria-label={link.title} elevation={8} onPointerEnter={keep} onPointerLeave={leave} sx={{ maxWidth: '92vw' }}>{details}</Paper>
                </Popper>
            )}
            {mode === 'pinned' && (phone
                ? <Drawer anchor="bottom" open onClose={closePinned} slotProps={{ paper: { role: 'dialog', 'aria-label': link.title, sx: { borderTopLeftRadius: (theme) => theme.spacing(mobile.sheetRadius), borderTopRightRadius: (theme) => theme.spacing(mobile.sheetRadius), pb: safeAreaBottom } } }}>{details}</Drawer>
                : <Popover open anchorEl={row} onClose={closePinned}
                    anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }} transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                    slotProps={{ paper: { role: 'dialog', 'aria-label': link.title, sx: { maxWidth: '92vw' } } }}>
                    {details}
                </Popover>)}
        </Stack>
    );
}

/** A thread's issues and PRs, in the thread's info rail. Absent when the
 * thread has touched none. */
export function ThreadLinksCard({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    const links = thread.links ?? [];
    if (links.length === 0) return null;
    return (
        <RailCard id="links" title={t('links.title')} aside={String(links.length)}>
            <Box sx={{ mx: -0.25 }}>
                {links.map((link) => <LinkRow key={`${link.repo}/${link.kind}/${link.number}`} link={link} threadId={thread.id} />)}
            </Box>
        </RailCard>
    );
}

export interface ProjectLink {
    link: ThreadLink;
    threadId: string;
    /** The title of the thread that opened or touched it. */
    owner: string;
}

/** Every issue and PR the project's threads touched, newest first, each with its thread. */
export function collectLinks(threads: Thread[]): ProjectLink[] {
    return threads
        .flatMap((th) => (th.links ?? []).map((link) => ({ link, threadId: th.id, owner: th.title })))
        .sort((a, b) => Date.parse(b.link.created_at) - Date.parse(a.link.created_at));
}
