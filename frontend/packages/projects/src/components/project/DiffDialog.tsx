import { useEffect, useState } from 'react';
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Skeleton, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { api, ApiError, unwrap, type Diff, type Thread } from '../../client';
import { DiffIcon, Merge } from '../icons';
import { useCopyFeedback } from '../../hooks/useCopyFeedback';
import { useNotify } from '../../hooks/useNotify';

const mono = { fontFamily: fontMono } as const;

/** A thread's changes against its base, with "Merge it". */
export function DiffDialog({ thread, open, onClose }: { thread: Thread; open: boolean; onClose: () => void }) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [diff, setDiff] = useState<Diff | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [blocked, setBlocked] = useState<{ error: string; command: string } | null>(null);
    const [busy, setBusy] = useState(false);
    const { copied, copy } = useCopyFeedback();

    useEffect(() => {
        if (!open) return;
        setDiff(null);
        setError(null);
        setBlocked(null);
        unwrap(api.GET('/api/v1/threads/{id}/diff', { params: { path: { id: thread.id } } })).then(setDiff).catch((e: Error) => setError(e.message));
    }, [open, thread.id]);

    const merge = async () => {
        setBusy(true);
        try {
            await unwrap(api.POST('/api/v1/threads/{id}/merge', { params: { path: { id: thread.id } } }));
            notify(t('thread.merged', { branch: thread.branch, base: thread.base_branch }), 'success');
            onClose();
        } catch (e) {
            if (e instanceof ApiError && e.status === 409 && (e.body as { command?: string })?.command) {
                setBlocked(e.body as { error: string; command: string });
            } else {
                notify((e as Error).message, 'error');
            }
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <DiffIcon />
                <Box component="span" sx={{ flex: 1 }}>{thread.title}</Box>
                <Typography component="span" sx={{ ...mono, fontSize: fontSizes.lg, color: 'text.secondary' }}>{thread.base_branch}…{thread.branch}</Typography>
            </DialogTitle>
            <DialogContent dividers>
                {error && <Alert severity="error">{error}</Alert>}
                {!diff && !error && <Skeleton variant="rounded" height={200} />}
                {diff && (
                    <>
                        {diff.uncommitted && <Alert severity="warning" sx={{ mb: 1.5 }}>{t('thread.uncommittedWarn')}<Box component="pre" sx={{ ...mono, fontSize: fontSizes.sm, m: 0, mt: 0.5 }}>{diff.uncommitted}</Box></Alert>}
                        <Box component="pre" sx={{ ...mono, fontSize: fontSizes.md, m: 0, mb: 1.5, color: 'text.secondary', whiteSpace: 'pre-wrap' }}>{diff.stat || t('thread.noChanges')}</Box>
                        <Box component="pre" sx={{ ...mono, fontSize: fontSizes.md, m: 0, p: 1.5, bgcolor: 'action.hover', borderRadius: 1, overflow: 'auto', maxHeight: '55vh' }}>
                            {diff.patch.split('\n').map((line, i) => (
                                <Box key={i} component="div" sx={{
                                    color: line.startsWith('+') && !line.startsWith('+++') ? 'success.main' : line.startsWith('-') && !line.startsWith('---') ? 'error.main' : line.startsWith('@@') ? 'text.secondary' : 'text.primary',
                                    fontWeight: line.startsWith('diff --git') ? 700 : 400,
                                }}>{line || ' '}</Box>
                            ))}
                        </Box>
                        {diff.truncated && <Typography variant="caption" color="text.secondary">{t('thread.truncated')}</Typography>}
                    </>
                )}
                {blocked && (
                    <Alert severity="warning" sx={{ mt: 1.5, '& .MuiAlert-message': { width: '100%' } }}>
                        <Typography variant="body2" sx={{ color: 'inherit', fontWeight: 600 }}>{t('thread.mergeBlocked')}: {blocked.error}</Typography>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 1, p: 1, bgcolor: 'background.paper', borderRadius: 1 }}>
                            <Box component="code" sx={{ ...mono, fontSize: fontSizes.md, flex: 1, wordBreak: 'break-all' }}>{blocked.command}</Box>
                            <Button size="small" onClick={() => copy(blocked.command)}>{copied ? t('common.copied') : t('common.copy')}</Button>
                        </Box>
                    </Alert>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>{t('thread.mergeHint', { base: thread.base_branch, repo: thread.repo_path })}</Typography>
                <Button onClick={onClose} color="inherit">{t('common.close')}</Button>
                <Button variant="contained" startIcon={<Merge />} disabled={busy || !diff || !diff.patch} onClick={() => void merge()}>{t('thread.mergeIt')}</Button>
            </DialogActions>
        </Dialog>
    );
}
