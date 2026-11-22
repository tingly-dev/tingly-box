import { useCallback, useId, useMemo, useRef, useState } from 'react';
import { Box, Collapse, IconButton, Link, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Thread } from '../../client';
import { describeOutcome, useLinkRefresh } from '../../hooks/useLinkRefresh';
import { useGroupFolds } from '../../hooks/useGroupFolds';
import { timeAgo } from '../../utils/format';
import { Refresh, Spinner } from '../icons';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { mobile } from '@tingly/ui/theme/mobile';
import { radius } from '@tingly/ui/theme/radius';
import { DisclosureRow } from '../common/Disclosure';
import { ListRow } from '../ListRow';
import { useFoldTimeout } from '../workspace/StatusGroups';
import { collectLinks, type ProjectLink } from './LinksCard';
import { colorOf, linkParts, LinkIcon, spin } from './linkView';
import { peekRowSx, useLinkPeek } from './useLinkPeek';

type Filter = 'open' | 'closed';

/** Whether an item is still open; merged and closed ones are finished. */
export const isOpenLink = ({ link }: ProjectLink) => link.state === 'open';

/** One item of the page, as a row of the Threads tab's list (`ListRow`): its
 * kind and state in the mark, `#12 title` as a link to GitHub, under it where
 * it stands and the thread it came from (a link that opens the thread), at the
 * right how old the reading is, then the row's refresh and details. */
function ItemRow({ item: { link, owner, threadId }, onOpen }: { item: ProjectLink; onOpen: (threadId: string) => void }) {
    const { t, i18n } = useTranslation('projects');
    const peek = useLinkPeek(link, threadId);
    const read = timeAgo(link.refreshed_at, i18n.language);
    return (
        <Box {...peek.row} sx={peekRowSx}>
            <ListRow
                mark={<LinkIcon link={link} />}
                title={<Link href={link.url.startsWith('https://') ? link.url : undefined} target="_blank" rel="noopener noreferrer" underline="hover" color="inherit" title={`${link.repo}#${link.number}`}>#{link.number} {link.title}</Link>}
                // Where it stands leads: a narrow pane clips the thread's title, never the checks.
                subtitle={<Box component="span" sx={{ display: 'flex', columnGap: 0.75, minWidth: 0 }}>
                    {linkParts(link, t).map((part) => <Box key={part.text} component="span" sx={{ flexShrink: 0, color: (theme) => colorOf(theme, part.tone) }}>{part.text}</Box>)}
                    {link.last_error && <Box component="span" title={link.last_error} sx={{ flexShrink: 0, color: (theme) => colorOf(theme, 'warning') }}>{t('links.stale', { age: read })}</Box>}
                    <Link component="button" type="button" underline="hover" color="text.secondary" onClick={() => onOpen(threadId)} title={t('links.openThread', { title: owner })}
                        sx={{ minWidth: 0, fontSize: 'inherit', textAlign: 'left', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{owner}</Link>
                </Box>}
                // Short, as the Library's: the pane is narrow and the title needs the room.
                meta={link.state === 'open' && !link.last_error && read ? <span title={`${t('links.readTitle')} · ${read}`}>{timeAgo(link.refreshed_at, i18n.language, true)}</span> : undefined}
                end={peek.actions}
                sx={{ py: 0.75 }}
            />
            {peek.overlay}
        </Box>
    );
}

/**
 * The project's issues and PRs on a page of their own, drawn the way the
 * Threads tab draws its threads: the same toolbar (Open | Closed and the
 * refresh where the count and the view switch are), then quiet folding groups,
 * PRs and issues, of `ListRow`s. Only the list scrolls; the toolbar stays put.
 */
export function ProjectLinks({ projectId, threads, onOpen }: { projectId: string; threads: Thread[]; onOpen: (threadId: string) => void }) {
    const { t } = useTranslation('projects');
    const call = useCallback(() => unwrap(api.POST('/api/v1/projects/{id}/links/refresh', { params: { path: { id: projectId } } })), [projectId]);
    const { busy, outcome, run } = useLinkRefresh(call);
    const said = describeOutcome(outcome, t);
    const [choice, setChoice] = useState<Filter | null>(null);
    const [folds, setFolds] = useGroupFolds(`${projectId}.links`);
    const timeout = useFoldTimeout();
    const base = useId();
    const scroller = useRef<HTMLDivElement>(null);
    // Every thread event re-renders the list; the sort only moves when a thread's links do.
    const rows = useMemo(() => collectLinks(threads), [threads]);
    const open = rows.filter(isOpenLink);
    const closed = rows.filter((r) => !isOpenLink(r));
    // Until the user picks, the page opens on what is open, or on what finished when nothing is: not on an empty list.
    const filter: Filter = choice ?? (open.length > 0 ? 'open' : 'closed');
    const shown = filter === 'open' ? open : closed;
    const pick = (v: Filter) => {
        // The other list starts at its top; a refresh or a thread event keeps the place.
        if (v !== filter && scroller.current) scroller.current.scrollTop = 0;
        setChoice(v);
    };
    const group = (key: string, label: string, list: ProjectLink[]) => {
        if (list.length === 0) return null;
        const unfolded = !folds[key];
        const bodyId = `${base}-${key}`;
        return (
            <Box component="section" aria-label={label} sx={{ mb: 1.5 }}>
                <DisclosureRow open={unfolded} onToggle={() => setFolds([key], unfolded)} aria-controls={bodyId} aria-label={`${label} ${list.length}`}
                    sx={{ gap: 1, py: 0.5, '@media (pointer: coarse)': { minHeight: mobile.tapMin } }}>
                    <Box component="span" sx={{ fontSize: fontSizes.sm, fontWeight: 600 }}>{label}</Box>
                    <Box component="span" sx={{ fontSize: fontSizes.sm, color: 'text.disabled' }}>{list.length}</Box>
                </DisclosureRow>
                <Collapse in={unfolded} timeout={timeout} id={bodyId}>
                    {list.map((item) => <ItemRow key={`${item.threadId}/${item.link.repo}/${item.link.kind}/${item.link.number}`} item={item} onOpen={onOpen} />)}
                </Collapse>
            </Box>
        );
    };
    const button = (value: Filter, label: string) => (
        <ToggleButton value={value} sx={{ px: 1, py: 0.25, border: 0, borderRadius: radius.row, fontSize: fontSizes.sm, textTransform: 'none', color: 'text.secondary', '&.Mui-selected': { color: 'text.primary', bgcolor: 'action.selected' } }}>{label}</ToggleButton>
    );
    const empty = rows.length === 0 ? t('links.empty') : shown.length === 0 ? t(filter === 'open' ? 'links.noneOpen' : 'links.noneClosed') : null;
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Box sx={{ flexShrink: 0, display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.75, minHeight: 40, borderBottom: 1, borderColor: 'divider' }}>
                <ToggleButtonGroup exclusive size="small" value={filter} aria-label={t('links.filter')} onChange={(_, v: Filter | null) => v && pick(v)} sx={{ gap: 0.25 }}>
                    {button('open', t('links.filterOpen', { count: open.length }))}
                    {button('closed', t('links.filterClosed', { count: closed.length }))}
                </ToggleButtonGroup>
                <Box sx={{ flex: 1 }} />
                {said && <Typography role="status" sx={{ fontSize: fontSizes.sm, color: 'text.secondary', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{said}</Typography>}
                {open.length > 0 && (
                    <IconButton size="small" aria-label={busy ? t('links.refreshing') : t('links.refreshAll')} title={t('links.refreshAll')} aria-busy={busy} disabled={busy} onClick={() => void run()} sx={{ color: 'text.secondary' }}>
                        {busy ? <Spinner sx={{ fontSize: 18, animation: `${spin} 1s linear infinite` }} /> : <Refresh sx={{ fontSize: 18 }} />}
                    </IconButton>
                )}
            </Box>
            {/* A plain block, not a flex column: a flex item that clips (a card, a Collapse) shrinks to the pane and cuts its own rows instead of letting the pane scroll. */}
            <Box ref={scroller} sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}>
                {empty != null
                    ? <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>{empty}</Typography>
                    : <Box sx={{ py: 1 }}>
                        {group('pr', t('links.prs'), shown.filter((r) => r.link.kind === 'pr'))}
                        {group('issue', t('links.issues'), shown.filter((r) => r.link.kind === 'issue'))}
                    </Box>}
            </Box>
        </Box>
    );
}
