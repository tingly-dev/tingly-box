import { useEffect, useState } from 'react';
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Link, Skeleton, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { api, unwrap, type Diff, type Thread, type ThreadLink } from '../../client';
import { DiffIcon, Merge } from '../icons';
import { MergeDialog } from './MergeDialog';
import { DiffView } from './diff/DiffView';

const mono = { fontFamily: fontMono } as const;

/** Whether a PR's changes can be shown: an open one, read from GitHub at least once (it is its branches that say what to diff). */
export const canShowFiles = (link: ThreadLink) => link.kind === 'pr' && link.state === 'open' && !!link.head && !!link.base;

/**
 * One PR's changes, head against its base, read from this checkout when it has
 * both branches and from GitHub when it does not. "Merge…" is here for the PR
 * of the thread's own branch: review, then merge.
 */
export function PrDiffDialog({ thread, link, open, onClose }: { thread: Thread; link: ThreadLink; open: boolean; onClose: () => void }) {
    const { t } = useTranslation('projects');
    const [diff, setDiff] = useState<Diff | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [merging, setMerging] = useState(false);
    const canMerge = link.head === thread.branch && !thread.merged_at;

    useEffect(() => {
        if (!open) return;
        setDiff(null);
        setError(null);
        setMerging(false);
        unwrap(api.GET('/api/v1/threads/{id}/prs/{number}/diff', { params: { path: { id: thread.id, number: link.number } } })).then(setDiff).catch((e: Error) => setError(e.message));
    }, [open, thread.id, link.number]);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="xl" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <DiffIcon />
                <Box component="span" sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>#{link.number} {link.title}</Box>
                <Typography component="span" sx={{ ...mono, fontSize: fontSizes.base, color: 'text.secondary', flexShrink: 0 }}>{link.base}…{link.head}</Typography>
            </DialogTitle>
            <DialogContent dividers>
                {error && <Alert severity="error">{error}</Alert>}
                {!diff && !error && <Skeleton variant="rounded" height={200} />}
                {diff && (
                    <>
                        {diff.source === 'github' && <Alert severity="info" sx={{ mb: 1.5 }}>{t('thread.prDiff.fromGitHub')}</Alert>}
                        {!!diff.unpushed && <Alert severity="info" sx={{ mb: 1.5 }}>{t('thread.prDiff.unpushed', { count: diff.unpushed })}</Alert>}
                        {diff.base_stale && diff.source === 'local' && <Alert severity="warning" sx={{ mb: 1.5 }}>{t('thread.prDiff.baseStale', { base: diff.base })}</Alert>}
                        <DiffView threadId={thread.id} number={link.number} diff={diff} />
                    </>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                {link.url.startsWith('https://') && <Link href={link.url} target="_blank" rel="noopener noreferrer" underline="hover" sx={{ fontSize: fontSizes.md }}>{t('links.openOnGitHub')}</Link>}
                <Box sx={{ flex: 1 }} />
                <Button onClick={onClose} color="inherit">{t('common.close')}</Button>
                {canMerge && <Button variant="contained" startIcon={<Merge />} onClick={() => setMerging(true)}>{t('thread.mergeIt')}</Button>}
            </DialogActions>
            {canMerge && <MergeDialog thread={thread} open={merging} onClose={() => setMerging(false)} onMerged={() => { setMerging(false); onClose(); }} />}
        </Dialog>
    );
}
