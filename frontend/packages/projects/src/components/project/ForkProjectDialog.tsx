import { useEffect, useState } from 'react';
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { api, unwrap, type Project } from '../../client';
import { useNotify } from '../../hooks/useNotify';
import { useProjectsHost } from '../../host/ProjectsHost';
import { Check, Close, Duplicate } from '../icons';

interface Props {
    project: Project;
    open: boolean;
    onClose: () => void;
    /** Called once the fork exists, before the page moves to it (e.g. to close the settings dialog under it). */
    onForked?: () => void;
}

/** What the copy carries and what it leaves behind, one quiet row each. */
function CopyList() {
    const { t } = useTranslation('projects');
    return (
        <Box sx={{ display: 'grid', gap: 0.75 }}>
            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                <Check sx={{ fontSize: 16, mt: '2px', color: 'primary.main' }} />
                <Typography variant="body2" color="text.primary">{t('projectSettings.forkCopies')}</Typography>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                <Close sx={{ fontSize: 16, mt: '2px', color: 'text.disabled' }} />
                <Typography variant="body2" color="text.secondary">{t('projectSettings.forkNotCopied')}</Typography>
            </Box>
        </Box>
    );
}

/**
 * Names the fork and says what it copies, then forks and opens the new
 * project. The copy runs synchronously; the confirm button is busy until the
 * server answers.
 */
export function ForkProjectDialog({ project, open, onClose, onForked }: Props) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const navigate = useNavigate();
    const { onProjectsChanged: refresh, paths: routes } = useProjectsHost();
    const [name, setName] = useState('');
    const [busy, setBusy] = useState(false);

    // Seed once per open (like NewProjectDialog seeds empty): a project event
    // arriving mid-edit must not wipe what is being typed.
    useEffect(() => {
        if (open) setName(`${project.name} (fork)`);
        // eslint-disable-next-line react-hooks/exhaustive-deps -- project.name is read at open only
    }, [open]);

    const submit = async () => {
        if (!name.trim() || busy) return;
        setBusy(true);
        try {
            const fork = await unwrap(api.POST('/api/v1/projects/{id}/fork', { params: { path: { id: project.id } }, body: { name: name.trim() } }));
            refresh();
            onForked?.();
            onClose();
            notify(t('projectSettings.forkDone', { name: fork.name }), 'success');
            navigate(routes.project(fork.id));
        } catch (e) {
            notify((e as Error).message, 'error');
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>{t('projectSettings.forkTitle')}</DialogTitle>
            <DialogContent>
                <Stack spacing={2.5} sx={{ pt: 1 }}>
                    <TextField
                        label={t('project.name')}
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        autoFocus
                        fullWidth
                        onKeyDown={(e) => e.key === 'Enter' && void submit()}
                    />
                    <CopyList />
                    <Typography variant="body2" color="text.secondary">{t('projectSettings.forkNameHelp')}</Typography>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} color="inherit">{t('common.cancel')}</Button>
                <Button variant="contained" startIcon={<Duplicate />} onClick={() => void submit()} disabled={!name.trim() || busy}>
                    {t('projectSettings.fork')}
                </Button>
            </DialogActions>
        </Dialog>
    );
}
