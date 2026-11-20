import { useState } from 'react';
import { Box, Button, IconButton, Link, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { Thread, ThreadLink } from '../../client';
import { timeAgo } from '../../utils/format';
import { DiffIcon, Merge } from '../icons';
import { RailCard } from './infoRail';
import { MergeDialog } from './MergeDialog';
import { canShowFiles, PrDiffDialog } from './PrDiffDialog';
import { colorOf, linkParts, LinkIcon } from './linkView';
import { peekRowSx, useLinkPeek } from './useLinkPeek';

/** The button that opens a PR's changes, beside its refresh and details buttons. */
export function FilesButton({ link, onClick }: { link: ThreadLink; onClick: () => void }) {
    const { t } = useTranslation('projects');
    return (
        <Tooltip title={t('links.files')}>
            <IconButton size="small" aria-label={t('links.filesOf', { ref: `${link.repo}#${link.number}` })} onClick={onClick}
                sx={{ p: 0.25, color: 'text.secondary', '@media (pointer: coarse)': { minWidth: 36, minHeight: 36 } }}>
                <DiffIcon sx={{ fontSize: 16 }} />
            </IconButton>
        </Tooltip>
    );
}

/** One issue or PR: its number and title as a link to GitHub, and under it
 * where it stands and how old that reading is. At the row's end, a refresh and
 * a details button (`useLinkPeek`), and, for a PR whose changes can be read,
 * the button that opens them (`onFiles`). */
export function LinkRow({ link, threadId, owner, onOpenOwner, onFiles }: { link: ThreadLink; threadId?: string; owner?: string; onOpenOwner?: () => void; onFiles?: () => void }) {
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
            <Box sx={{ mt: '-2px', display: 'flex', alignItems: 'center' }}>
                {onFiles && <FilesButton link={link} onClick={onFiles} />}
                {peek.actions}
            </Box>
            {peek.overlay}
        </Stack>
    );
}

/** A thread's issues, in the thread's info rail. Absent when it touched none. */
export function ThreadIssuesCard({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    const issues = (thread.links ?? []).filter((l) => l.kind === 'issue');
    if (issues.length === 0) return null;
    return (
        <RailCard id="issues" title={t('links.issues')} aside={String(issues.length)}>
            <Box sx={{ mx: -0.25 }}>
                {issues.map((link) => <LinkRow key={`${link.repo}/${link.kind}/${link.number}`} link={link} threadId={thread.id} />)}
            </Box>
        </RailCard>
    );
}

/**
 * A thread's PRs, in the thread's info rail: each with its own changes one
 * button away. A branch with commits and no open PR has one row of its own: how
 * far it is ahead, and "Merge…" (open a PR, or squash it) in place of the PR it
 * does not have yet.
 */
export function ThreadPrsCard({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    const prs = (thread.links ?? []).filter((l) => l.kind === 'pr');
    const [showing, setShowing] = useState<ThreadLink | null>(null);
    const [merging, setMerging] = useState(false);
    const hasOpenPr = prs.some((l) => l.state === 'open' && l.head === thread.branch);
    const canOpen = !!thread.branch && thread.commits_ahead > 0 && !thread.merged_at && !hasOpenPr;
    const mergedWithoutPr = !!thread.merged_at && prs.length === 0;
    if (prs.length === 0 && !canOpen && !mergedWithoutPr) return null;
    return (
        <RailCard id="prs" title={t('links.prs')} aside={prs.length > 0 ? String(prs.length) : undefined}>
            <Box sx={{ mx: -0.25 }}>
                {prs.map((link) => <LinkRow key={`${link.repo}/${link.kind}/${link.number}`} link={link} threadId={thread.id} onFiles={canShowFiles(link) ? () => setShowing(link) : undefined} />)}
            </Box>
            {canOpen && (
                <Box sx={{ pt: prs.length > 0 ? 0.75 : 0, borderTop: prs.length > 0 ? 1 : 0, borderColor: 'divider' }}>
                    <Typography sx={{ fontSize: fontSizes.sm, color: 'text.primary' }}>
                        {t('links.noPr', { base: thread.base_branch })} · {t('thread.commits', { count: thread.commits_ahead })} · {t('thread.files', { count: thread.files_changed })}
                    </Typography>
                    {thread.uncommitted && <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('thread.uncommitted')}</Typography>}
                    <Button size="small" variant="outlined" startIcon={<Merge />} onClick={() => setMerging(true)} sx={{ mt: 0.75 }}>{t('thread.mergeIt')}</Button>
                    <MergeDialog thread={thread} open={merging} onClose={() => setMerging(false)} onMerged={() => setMerging(false)} />
                </Box>
            )}
            {mergedWithoutPr && <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('thread.mergedInto', { base: thread.base_branch })}</Typography>}
            {showing && <PrDiffDialog thread={thread} link={showing} open onClose={() => setShowing(null)} />}
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
