import { useEffect, useState } from 'react';
import { Alert, Button, Dialog, DialogActions, DialogContent, TextField, Typography, Box } from '@mui/material';
import { DialogHeader } from '@tingly/ui';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Thread } from '../../client';
import { useNotify } from '../../hooks/useNotify';
import { readLocal, writeLocal } from '../../utils/storage';
import { Merge, PullRequest } from '../icons';
import { ChoiceList, type Choice } from '../im/ChoiceList';

export type MergeMethod = 'squash' | 'merge_request';

const METHODS: MergeMethod[] = ['squash', 'merge_request'];
const methodKey = (projectId: string) => `merge-method.${projectId}`;

/** The method this project last used; a squash until one was chosen. */
export function rememberedMethod(projectId: string): MergeMethod {
    const saved = readLocal(methodKey(projectId));
    return METHODS.find((m) => m === saved) ?? 'squash';
}

/** The open PR of the thread's own branch, if its tools or an earlier merge request opened one. */
export function openMergeRequest(thread: Thread) {
    return (thread.links ?? []).find((l) => l.kind === 'pr' && l.state === 'open' && l.head === thread.branch);
}

/**
 * How a thread's branch is merged: squashed into its base in the checkout, or
 * offered as a PR from the pushed branch. Exactly these two, the last pick
 * remembered per project.
 */
export function MergeDialog({ thread, open, onClose, onMerged }: { thread: Thread; open: boolean; onClose: () => void; onMerged: () => void }) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [method, setMethod] = useState<MergeMethod>('squash');
    const [message, setMessage] = useState('');
    const [title, setTitle] = useState('');
    const [body, setBody] = useState('');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const existing = openMergeRequest(thread);

    useEffect(() => {
        if (!open) return;
        setMethod(rememberedMethod(thread.project_id));
        setMessage(thread.title);
        setTitle(thread.title);
        setBody(thread.last_report ?? '');
        setError(null);
    }, [open, thread.project_id, thread.title, thread.last_report]);

    const choices: Choice[] = [
        { key: 'squash', lead: <Merge sx={{ fontSize: 22, color: 'text.secondary' }} />, title: t('thread.merge.squash'), subtitle: t('thread.merge.squashHint', { base: thread.base_branch }) },
        { key: 'merge_request', lead: <PullRequest sx={{ fontSize: 22, color: 'text.secondary' }} />, title: t('thread.merge.mr'), subtitle: t('thread.merge.mrHint', { base: thread.base_branch }) },
    ];

    const submit = async () => {
        setBusy(true);
        setError(null);
        try {
            const req = method === 'squash' ? { method, message } : { method, title, body };
            await unwrap(api.POST('/api/v1/threads/{id}/merge', { params: { path: { id: thread.id } }, body: req }));
            writeLocal(methodKey(thread.project_id), method);
            notify(method === 'squash'
                ? t('thread.merged', { branch: thread.branch, base: thread.base_branch })
                : t(existing ? 'thread.merge.mrKept' : 'thread.merge.mrOpened', { branch: thread.branch, base: thread.base_branch }), 'success');
            onMerged();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    const canSubmit = method === 'squash' ? message.trim() !== '' : !!existing || title.trim() !== '';
    return (
        <Dialog open={open} onClose={busy ? undefined : onClose} maxWidth="sm" fullWidth aria-labelledby="merge-title">
            <DialogHeader titleId="merge-title" title={t('thread.merge.title')} closeLabel={t('common.close')} onClose={onClose} closeDisabled={busy} />
            <DialogContent sx={{ pt: 0.5 }}>
                <ChoiceList items={choices} value={method} onChange={(k) => setMethod(k as MergeMethod)} label={t('thread.merge.title')} />
                <Box sx={{ mt: 2, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                    {method === 'squash' && (
                        <TextField label={t('thread.merge.message')} value={message} onChange={(e) => setMessage(e.target.value)} multiline minRows={2} maxRows={8} fullWidth size="small" />
                    )}
                    {method === 'merge_request' && existing && (
                        <Alert severity="info">{t('thread.merge.mrExisting', { number: existing.number })}</Alert>
                    )}
                    {method === 'merge_request' && !existing && (
                        <>
                            <TextField label={t('thread.merge.prTitle')} value={title} onChange={(e) => setTitle(e.target.value)} fullWidth size="small" />
                            <TextField label={t('thread.merge.prBody')} value={body} onChange={(e) => setBody(e.target.value)} multiline minRows={3} maxRows={8} fullWidth size="small" />
                        </>
                    )}
                    <Typography variant="caption" color="text.secondary">
                        {t(method === 'squash' ? 'thread.merge.squashNote' : 'thread.merge.mrNote', { branch: thread.branch, base: thread.base_branch })}
                    </Typography>
                    {error && <Alert severity="error">{error}</Alert>}
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} color="inherit" disabled={busy}>{t('common.cancel')}</Button>
                <Button variant="contained" startIcon={method === 'squash' ? <Merge /> : <PullRequest />} disabled={busy || !canSubmit} onClick={() => void submit()}>
                    {method === 'squash' ? t('thread.merge.doSquash') : t(existing ? 'thread.merge.doPush' : 'thread.merge.doOpen')}
                </Button>
            </DialogActions>
        </Dialog>
    );
}
