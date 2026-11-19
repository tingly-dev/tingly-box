import React, { useState } from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';

interface DuplicateDialogProps {
    open: boolean;
    onClose: () => void;
    title: string;
    description?: React.ReactNode;
    /** Label of the name field. */
    label: string;
    /** A free name, prefilled so a plain confirm already works. */
    initialName: string;
    /** Resolves true when the copy was made (the dialog closes); false keeps it open to fix the name. */
    onSubmit: (name: string) => Promise<boolean>;
}

// Names the copy of a profile or Team. Shared by every "Duplicate" action so
// they behave alike: prefilled free name, Enter confirms, stays open on failure.
const DuplicateDialog: React.FC<DuplicateDialogProps> = ({ open, onClose, title, description, label, initialName, onSubmit }) => {
    const { t } = useTranslation();
    const [name, setName] = useState(initialName);
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        if (!name.trim() || busy) return;
        setBusy(true);
        try {
            if (await onSubmit(name.trim())) onClose();
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog
            open={open}
            onClose={busy ? undefined : onClose}
            maxWidth="xs"
            fullWidth
            // Each opening starts from the current suggestion, not from the last edit.
            slotProps={{ transition: { onEnter: () => setName(initialName) } }}
        >
            <DialogTitle>{title}</DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                {description && (
                    <Typography variant="body2" color="text.secondary">{description}</Typography>
                )}
                <TextField
                    autoFocus
                    fullWidth
                    label={label}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && submit()}
                    size="small"
                    margin="dense"
                />
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                <Button onClick={onClose} color="inherit" disabled={busy}>
                    {t('common.cancel')}
                </Button>
                <Button onClick={submit} variant="contained" disabled={!name.trim() || busy}>
                    {t('common.duplicate')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default DuplicateDialog;
