import { useEffect, useState } from 'react';
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Skeleton, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { api, unwrap, type Diff, type Thread } from '../../client';
import { DiffIcon, Merge } from '../icons';
import { MergeDialog } from './MergeDialog';
import { radius } from '@tingly/ui/theme/radius';
import { DiffView } from './diff/DiffView';

const mono = { fontFamily: fontMono } as const;

/** A thread's changes against its base, with "Merge…" (squash or merge request, on GitHub). */
export function DiffDialog({ thread, open, onClose }: { thread: Thread; open: boolean; onClose: () => void }) {
    const { t } = useTranslation('projects');
    const [diff, setDiff] = useState<Diff | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [merging, setMerging] = useState(false);

    useEffect(() => {
        if (!open) return;
        setDiff(null);
        setError(null);
        setMerging(false);
        unwrap(api.GET('/api/v1/threads/{id}/diff', { params: { path: { id: thread.id } } })).then(setDiff).catch((e: Error) => setError(e.message));
    }, [open, thread.id]);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="xl" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <DiffIcon />
                <Box component="span" sx={{ flex: 1 }}>{thread.title}</Box>
                <Typography component="span" sx={{ ...mono, fontSize: fontSizes.base, color: 'text.secondary' }}>{thread.base_branch}…{thread.branch}</Typography>
            </DialogTitle>
            <DialogContent dividers>
                {error && <Alert severity="error">{error}</Alert>}
                {!diff && !error && <Skeleton variant="rounded" height={200} />}
                {diff && (
                    <>
                        {diff.uncommitted && <Alert severity="warning" sx={{ mb: 1.5 }}>{t('thread.uncommittedWarn')}<Box component="pre" sx={{ ...mono, fontSize: fontSizes.sm, m: 0, mt: 0.5 }}>{diff.uncommitted}</Box></Alert>}
                        <DiffView threadId={thread.id} diff={diff} />
                    </>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                <Box sx={{ flex: 1 }} />
                <Button onClick={onClose} color="inherit">{t('common.close')}</Button>
                <Button variant="contained" startIcon={<Merge />} disabled={!diff || !diff.files.length} onClick={() => setMerging(true)}>{t('thread.mergeIt')}</Button>
            </DialogActions>
            <MergeDialog thread={thread} open={merging} onClose={() => setMerging(false)} onMerged={() => { setMerging(false); onClose(); }} />
        </Dialog>
    );
}
