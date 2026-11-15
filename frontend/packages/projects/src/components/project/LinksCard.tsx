import { Box, Link, Stack } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { Thread, ThreadLink } from '../../client';
import { timeAgo } from '../../utils/format';
import { RailCard } from './infoRail';
import { colorOf, linkParts, LinkIcon } from './linkView';
import { peekRowSx, useLinkPeek } from './useLinkPeek';

/** One issue or PR: its number and title as a link to GitHub, and under it
 * where it stands and how old that reading is. At the row's end, a refresh and
 * a details button (`useLinkPeek`). */
export function LinkRow({ link, threadId, owner, onOpenOwner }: { link: ThreadLink; threadId?: string; owner?: string; onOpenOwner?: () => void }) {
    const { t, i18n } = useTranslation('projects');
    const parts = linkParts(link, t);
    const age = timeAgo(link.refreshed_at, i18n.language);
    const peek = useLinkPeek(link, threadId);
    return (
        <Stack {...peek.row} direction="row" spacing={0.75} sx={{ alignItems: 'flex-start', minWidth: 0, py: 0.625, borderTop: 1, borderColor: 'divider', ...peekRowSx }}>
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
            <Box sx={{ mt: '-2px', display: 'flex' }}>{peek.actions}</Box>
            {peek.overlay}
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
