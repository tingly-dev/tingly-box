import { useMemo, useState } from 'react';
import { Box, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { collectLinks, LinkRow, type ProjectLink } from './LinksCard';
import { RailCard } from './infoRail';

type Filter = 'open' | 'closed';

/** Whether an item is still open; merged and closed ones are finished. */
export const isOpenLink = ({ link }: ProjectLink) => link.state === 'open';

/**
 * The project's issues and PRs on a page of their own: what the project's
 * threads opened or touched, each with the thread it came from. PRs and issues
 * sit in cards of their own; Open | Closed is the page's one control.
 */
export function ProjectLinks({ threads, onOpen }: { threads: Thread[]; onOpen: (threadId: string) => void }) {
    const { t } = useTranslation('projects');
    const [choice, setChoice] = useState<Filter | null>(null);
    // Every thread event re-renders the list; the sort only moves when a thread's links do.
    const rows = useMemo(() => collectLinks(threads), [threads]);
    const open = rows.filter(isOpenLink);
    const closed = rows.filter((r) => !isOpenLink(r));
    // Until the user picks, the page opens on what is open, or on what finished when nothing is: not on an empty list.
    const filter: Filter = choice ?? (open.length > 0 ? 'open' : 'closed');
    const shown = filter === 'open' ? open : closed;
    const prs = shown.filter((r) => r.link.kind === 'pr');
    const issues = shown.filter((r) => r.link.kind === 'issue');
    const section = (id: string, title: string, list: ProjectLink[]) => list.length > 0 && (
        <RailCard id={id} title={title} aside={String(list.length)}>
            <Box sx={{ mx: -0.25 }}>
                {list.map(({ link, owner, threadId }) => <LinkRow key={`${threadId}/${link.repo}/${link.kind}/${link.number}`} link={link} owner={owner} onOpenOwner={() => onOpen(threadId)} />)}
            </Box>
        </RailCard>
    );
    const button = (value: Filter, label: string) => (
        <ToggleButton value={value} sx={{ px: 1, py: 0.25, border: 0, borderRadius: radius.row, fontSize: fontSizes.sm, textTransform: 'none', color: 'text.secondary', '&.Mui-selected': { color: 'text.primary', bgcolor: 'action.selected' } }}>{label}</ToggleButton>
    );
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.75, minHeight: 40, borderBottom: 1, borderColor: 'divider' }}>
                <ToggleButtonGroup exclusive size="small" value={filter} aria-label={t('links.filter')} onChange={(_, v: Filter | null) => v && setChoice(v)} sx={{ gap: 0.25 }}>
                    {button('open', t('links.filterOpen', { count: open.length }))}
                    {button('closed', t('links.filterClosed', { count: closed.length }))}
                </ToggleButtonGroup>
            </Box>
            <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden', p: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
                {rows.length === 0 && <Typography variant="body2" color="text.secondary">{t('links.empty')}</Typography>}
                {rows.length > 0 && shown.length === 0 && <Typography variant="body2" color="text.secondary">{t(filter === 'open' ? 'links.noneOpen' : 'links.noneClosed')}</Typography>}
                {section('project-prs', t('links.prs'), prs)}
                {section('project-issues', t('links.issues'), issues)}
            </Box>
        </Box>
    );
}
