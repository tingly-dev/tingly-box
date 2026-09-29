import React, { useState } from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';

export interface DuplicateResult {
    id: string;
    name: string;
    scenario: string;
}

// DuplicateDialog copies a team or profile (`sourceId` under `scenario`, or
// the main scope with 'default') into a new one named by the user, then hands
// the result to onDuplicated — typically to navigate to the copy. It stays
// open on failure so the name can be fixed.
export const DuplicateDialog: React.FC<{
    open: boolean;
    onClose: () => void;
    scenario: string;
    sourceId: string;
    defaultName: string;
    title: string;
    hint: string;
    nameLabel: string;
    onDuplicated: (result: DuplicateResult) => void | Promise<void>;
}> = ({ open, onClose, ...props }) => (
    // The body mounts per open, so the name starts from defaultName each time.
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
        <DuplicateDialogBody onClose={onClose} {...props} />
    </Dialog>
);

const DuplicateDialogBody: React.FC<Omit<React.ComponentProps<typeof DuplicateDialog>, 'open'>> = ({
    onClose, scenario, sourceId, defaultName, title, hint, nameLabel, onDuplicated,
}) => {
    const { t } = useTranslation();
    const notify = useNotify();
    const [name, setName] = useState(defaultName);
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        if (!name.trim() || busy) return;
        setBusy(true);
        const result = await api.duplicateScope(scenario, sourceId, name.trim());
        if (result?.success) {
            await onDuplicated(result.data);
            onClose();
        } else {
            notify.error(result?.error || 'Duplicate failed');
        }
        setBusy(false);
    };

    return (
        <>
            <DialogTitle>{title}</DialogTitle>
            <DialogContent>
                <Typography variant="body2" sx={{ color: 'text.secondary', mb: 1 }}>{hint}</Typography>
                <TextField
                    autoFocus
                    fullWidth
                    size="small"
                    margin="dense"
                    label={nameLabel}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && void submit()}
                    disabled={busy}
                />
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} color="inherit" disabled={busy}>{t('common.cancel')}</Button>
                <Button variant="contained" onClick={() => void submit()} disabled={busy || !name.trim()}>
                    {t('common.duplicate')}
                </Button>
            </DialogActions>
        </>
    );
};

export default DuplicateDialog;
