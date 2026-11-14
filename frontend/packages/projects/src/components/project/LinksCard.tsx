import { Box, Link, Stack } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Theme } from '@mui/material/styles';
import { getReadableAccent, type StatusTone } from '@tingly/ui/theme/status';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { Thread, ThreadLink } from '../../client';
import { IssueOpened, Merge, PullRequest } from '../icons';
import { timeAgo } from '../../utils/format';
import { RailCard } from './infoRail';

type Tone = Extract<StatusTone, 'success' | 'info' | 'warning' | 'error' | 'muted'>;

interface Part {
    text: string;
    tone: Tone;
}

const colorOf = (theme: Theme, tone: Tone) => (tone === 'muted' ? theme.palette.text.secondary : getReadableAccent(theme, tone));

/** Where an item stands: open, merged or closed, and for an open PR its draft
 * mark, checks and mergeability. Only what GitHub last said; a field it was
 * not asked about yet (a PR just opened) is left out, not shown as unknown. */
function linkParts(link: ThreadLink, t: (key: string) => string): Part[] {
    const parts: Part[] = [];
    if (link.state === 'merged') return [{ text: t('links.merged'), tone: 'info' }];
    if (link.state === 'closed') return [{ text: t('links.closed'), tone: 'muted' }];
    parts.push({ text: link.draft ? t('links.draft') : t('links.open'), tone: link.draft ? 'muted' : 'success' });
    if (link.kind !== 'pr') return parts;
    if (link.ci === 'failing') parts.push({ text: t('links.checksFailing'), tone: 'error' });
    else if (link.ci === 'pending') parts.push({ text: t('links.checksPending'), tone: 'warning' });
    else if (link.ci === 'passing') parts.push({ text: t('links.checksPassing'), tone: 'success' });
    if (link.merge_state === 'conflicts') parts.push({ text: t('links.conflicts'), tone: 'error' });
    else if (link.merge_state === 'behind') parts.push({ text: t('links.behind'), tone: 'warning' });
    else if (link.merge_state === 'blocked') parts.push({ text: t('links.blocked'), tone: 'warning' });
    else if (link.merge_state === 'ready') parts.push({ text: t('links.ready'), tone: 'success' });
    return parts;
}

function LinkIcon({ link }: { link: ThreadLink }) {
    const sx = { fontSize: 16, flexShrink: 0, mt: '2px' } as const;
    if (link.kind === 'issue') return <IssueOpened sx={{ ...sx, color: (theme) => colorOf(theme, link.state === 'open' ? 'success' : 'muted') }} />;
    if (link.state === 'merged') return <Merge sx={{ ...sx, color: (theme) => colorOf(theme, 'info') }} />;
    const tone: Tone = link.state === 'closed' ? 'error' : link.draft ? 'muted' : 'success';
    return <PullRequest sx={{ ...sx, color: (theme) => colorOf(theme, tone) }} />;
}

/** One issue or PR: its number and title as a link to GitHub, and under it
 * where it stands and how old that reading is. */
export function LinkRow({ link, owner, onOpenOwner }: { link: ThreadLink; owner?: string; onOpenOwner?: () => void }) {
    const { t, i18n } = useTranslation('projects');
    const parts = linkParts(link, t);
    const age = timeAgo(link.refreshed_at, i18n.language);
    return (
        <Stack direction="row" spacing={0.75} sx={{ alignItems: 'flex-start', minWidth: 0, py: 0.625, borderTop: 1, borderColor: 'divider' }}>
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
                {links.map((link) => <LinkRow key={`${link.repo}/${link.kind}/${link.number}`} link={link} />)}
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
