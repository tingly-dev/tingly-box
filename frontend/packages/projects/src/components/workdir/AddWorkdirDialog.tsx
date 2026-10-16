import { useState } from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono } from '@tingly/ui/theme/fonts';
import { api, unwrap, type Workdir } from '../../client';

/** Keeps an existing folder in the sidebar; the server checks that it is one. */
export function AddWorkdirDialog({ onClose, onAdded }: { onClose: () => void; onAdded: (w: Workdir) => void }) {
    const { t } = useTranslation('projects');
    const [value, setValue] = useState('');
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        setBusy(true);
        try {
            const w = await unwrap(api.POST('/api/v1/projects/workdirs', { body: { path: value.trim() } }));
            onAdded(w);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open onClose={onClose} maxWidth="sm" fullWidth aria-labelledby="add-workdir-title">
            <form onSubmit={(e) => { e.preventDefault(); if (value.trim() && !busy) void submit(); }}>
                <DialogTitle id="add-workdir-title">{t('workdir.add')}</DialogTitle>
                <DialogContent>
                    <DialogContentText sx={{ mb: 2 }}>{t('workdir.addHelp')}</DialogContentText>
                    <TextField
                        autoFocus fullWidth label={t('session.folder')}
                        placeholder={t('workdir.pathExample')}
                        value={value}
                        onChange={(e) => { setValue(e.target.value); setError(''); }}
                        error={!!error}
                        helperText={error || undefined}
                        slotProps={{ htmlInput: { style: { fontFamily: fontMono } } }}
                    />
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={onClose}>{t('common.cancel')}</Button>
                    <Button type="submit" variant="contained" disabled={!value.trim() || busy}>{t('workdir.add')}</Button>
                </DialogActions>
            </form>
        </Dialog>
    );
}
