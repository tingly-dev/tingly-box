import { useEffect, useState } from 'react';
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { RepoField, type RepoEntry } from './RepoField';

interface Props {
    open: boolean;
    onClose: () => void;
    onCreate: (v: { name: string; goal: string; repos: string[] }) => Promise<void>;
}

/** Only the name is required: a project can be created first and filled in
 * as the work takes shape. */
export function NewProjectDialog({ open, onClose, onCreate }: Props) {
    const { t } = useTranslation('projects');
    const [name, setName] = useState('');
    const [goal, setGoal] = useState('');
    const [repos, setRepos] = useState<RepoEntry[]>([]);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (open) {
            setName('');
            setGoal('');
            setRepos([]);
        }
    }, [open]);

    const submit = async () => {
        if (!name.trim()) return;
        setBusy(true);
        try {
            await onCreate({ name: name.trim(), goal: goal.trim(), repos: repos.map((r) => r.root) });
            onClose();
        } catch {
            // the caller already reported it
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>{t('projects.new')}</DialogTitle>
            <DialogContent>
                <Stack spacing={2.5} sx={{ pt: 1 }}>
                    <TextField
                        label={t('project.name')}
                        placeholder={t('project.namePlaceholder')}
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        autoFocus
                        fullWidth
                        onKeyDown={(e) => e.key === 'Enter' && void submit()}
                        helperText={t('project.nameHelp')}
                    />
                    <TextField
                        label={t('project.goal')}
                        placeholder={t('project.goalPlaceholder')}
                        value={goal}
                        onChange={(e) => setGoal(e.target.value)}
                        fullWidth
                        helperText={t('project.goalHelp')}
                    />
                    <Box>
                        <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary', mb: 0.25 }}>{t('project.repos')}</Typography>
                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>{t('project.reposHelp')}</Typography>
                        <RepoField value={repos} onChange={setRepos} />
                    </Box>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} color="inherit">{t('common.cancel')}</Button>
                <Button variant="contained" onClick={() => void submit()} disabled={!name.trim() || busy}>{t('project.create')}</Button>
            </DialogActions>
        </Dialog>
    );
}
